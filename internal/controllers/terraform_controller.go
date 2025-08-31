/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	errorscore "errors"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/go-logr/logr"
	"github.com/rinswind/terraform-operator/api/v1alpha1"
	"github.com/rinswind/terraform-operator/internal/metrics"
	"github.com/rinswind/terraform-operator/internal/terraform"
)

// TerraformReconciler reconciles a Terraform object
type TerraformReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	MetricsRecorder   metrics.RecorderInterface
	Log               logr.Logger
	requeueDependency time.Duration
	requeueJobWatch   time.Duration
}

// TerraformReconcilerOptions holds additional options
type TerraformReconcilerOptions struct {
	RequeueDependencyInterval time.Duration
	RequeueJobWatchInterval   time.Duration
}

//+kubebuilder:rbac:groups=run.terraform-operator.io,resources=terraforms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=run.terraform-operator.io,resources=terraforms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=run.terraform-operator.io,resources=terraforms/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Terraform object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *TerraformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	durationMsg := fmt.Sprintf("reconcilation finished in %s", time.Since(start).String())

	run := &v1alpha1.Terraform{}

	if err := r.Get(ctx, req.NamespacedName, run); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Wrap the raw resource in a helper that makes it easier to work with it and it's associated resources
	t := &terraform.TerraformManipulator{Terraform: run}

	if !controllerutil.ContainsFinalizer(t, v1alpha1.TerraformFinalizer) {
		controllerutil.AddFinalizer(t, v1alpha1.TerraformFinalizer)

		if err := r.Update(ctx, t.Terraform); err != nil {
			r.Log.Error(err, "unable to register finalizer")
			return ctrl.Result{}, err
		}

		r.Recorder.Event(t, corev1.EventTypeNormal, "Added finalizer", "Object finalizer is added")

		return ctrl.Result{}, nil
	}

	// Examine if the object is under deletion
	if !t.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleRunDelete(ctx, t)
	}

	if t.IsSubmitted() || t.IsWaiting() {
		result, err := r.handleRunCreate(ctx, t)
		if err != nil {
			return ctrl.Result{}, err
		}

		r.Recorder.Event(t, "Normal", "Created", fmt.Sprintf("Run(%s) submitted", t.Status.RunID))
		r.MetricsRecorder.RecordTotal(t.Name, t.Namespace)

		if result.RequeueAfter > 0 {
			r.Log.Info(fmt.Sprintf("%s, next run in %s", durationMsg, result.RequeueAfter.String()))
			return result, nil
		}

		return result, nil
	}

	if t.IsStarted() {
		result, err := r.handleRunJobWatch(ctx, t)
		if err != nil {
			return ctrl.Result{}, err
		}

		if result.RequeueAfter > 0 {
			r.Log.Info(fmt.Sprintf("%s, next run in %s", durationMsg, result.RequeueAfter.String()))
			return result, nil
		}

		return result, nil
	}

	if t.IsUpdated() {
		r.Log.Info("updating a terraform run")

		result, err := r.handleRunUpdate(ctx, t)
		if err != nil {
			return ctrl.Result{}, err
		}

		if result.RequeueAfter > 0 {
			r.Log.Info(fmt.Sprintf("%s, next run in %s", durationMsg, result.RequeueAfter.String()))
			return result, nil
		}

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager and configures
// the owned resources (Jobs, ConfigMaps, Secrets) that will trigger reconcile
// events when they change state. This establishes the controller's watch
// relationships and sets reconciler options for requeue intervals.
func (r *TerraformReconciler) SetupWithManager(mgr ctrl.Manager, opts TerraformReconcilerOptions) error {
	r.requeueDependency = opts.RequeueDependencyInterval
	r.requeueJobWatch = opts.RequeueJobWatchInterval

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Terraform{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

// handleRunCreate handles the creation of a new Terraform run. It checks dependencies,
// waits for them to complete if necessary, sets variables from dependencies,
// creates the Terraform run job, cleans up old resources, and updates the run status.
func (r *TerraformReconciler) handleRunCreate(ctx context.Context, t *terraform.TerraformManipulator) (ctrl.Result, error) {
	dependencies, err := t.CheckDependencies(ctx, r.Client)

	if err != nil {
		if t.IsWaiting() {
			return ctrl.Result{RequeueAfter: r.requeueDependency}, nil
		}

		r.Recorder.Event(t, "Normal", "Waiting", "Dependencies are not yet completed")

		// Always bail out after updating the status
		err := r.updateRunStatus(ctx, t, v1alpha1.RunWaitingForDependency)
		return ctrl.Result{RequeueAfter: r.requeueDependency}, err
	}

	t.SetVariablesFromDependencies(dependencies)

	_, err = t.CreateTerraformRun(ctx, r.Client)
	if err != nil {
		r.Log.Error(err, "failed create a terraform run")

		// Always bail out after updating the status
		if err := r.updateRunStatus(ctx, t, v1alpha1.RunFailed); err != nil {
			r.Log.Error(err, "failed to update status", "name", t.ObjectMeta.Name, "namespace", t.ObjectMeta.Namespace, "status", v1alpha1.RunFailed)
		}
		return ctrl.Result{}, err
	}

	r.Log.Info("cleaning up old resources if exist")

	if err = t.CleanupResources(ctx, r.Client); err != nil {
		r.Log.Error(err, "failed to cleanup resources")
	}

	// Always bail out after updating the status
	err = r.updateRunStatus(ctx, t, v1alpha1.RunStarted)
	return ctrl.Result{}, err
}

// handleRunUpdate handles updates to an existing Terraform run by creating a new run job.
// This method is called when a Terraform resource's generation changes, indicating
// the spec has been updated and needs to be reconciled.
func (r *TerraformReconciler) handleRunUpdate(ctx context.Context, t *terraform.TerraformManipulator) (ctrl.Result, error) {
	r.Recorder.Event(t, "Normal", "Updated", "Creating a new run job")

	return r.handleRunCreate(ctx, t)
}

// handleRunDelete handles the deletion of a Terraform resource by cleaning up finalizers.
// This method is called when a Terraform resource is marked for deletion, allowing for
// graceful cleanup of resources and metrics recording before removal.
func (r *TerraformReconciler) handleRunDelete(ctx context.Context, t *terraform.TerraformManipulator) (ctrl.Result, error) {
	r.Log.Info("terraform run is being deleted", "name", t.Name)

	r.MetricsRecorder.RecordStatus(t.Name, t.Namespace, v1alpha1.RunDeleted)
	controllerutil.RemoveFinalizer(t, v1alpha1.TerraformFinalizer)

	if err := r.Update(ctx, t.Terraform); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// handleRunJobWatch monitors the status of a Terraform job and updates the run status accordingly.
// It checks if the job is still running, has succeeded, or has failed, and takes appropriate actions
// such as cleaning up completed jobs, recording metrics, and updating the Terraform resource status.
func (r *TerraformReconciler) handleRunJobWatch(ctx context.Context, t *terraform.TerraformManipulator) (ctrl.Result, error) {
	job, err := t.GetJobForRun(ctx, r.Client, t.Status.RunID)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.Log.Info("waiting for terraform job run to complete", "name", job.Name)

	startTime, err := time.Parse(time.UnixDate, t.Status.StartedTime)
	if err != nil {
		r.Log.Error(err, "failed to parse workflow start time")
	}

	defer r.MetricsRecorder.RecordDuration(t.Name, t.Namespace, startTime)

	// job is still running
	if job.Status.Active > 0 {
		if t.IsRunning() {
			return ctrl.Result{RequeueAfter: r.requeueJobWatch}, nil
		}

		r.Recorder.Event(t, "Normal", "Running", fmt.Sprintf("Run(%s) waiting for run job to finish", t.Status.RunID))

		// Always bail out after updating the status
		err := r.updateRunStatus(ctx, t, v1alpha1.RunRunning)
		return ctrl.Result{}, err
	}

	// job is successful
	if job.Status.Succeeded > 0 {
		r.Log.Info("terraform run job completed successfully")

		if t.Spec.DeleteCompletedJobs {
			r.Log.Info("deleting completed job")

			if err := t.DeleteAfterCompletion(ctx, r.Client); err != nil {
				r.Log.Error(err, "failed to delete terraform run job after completion", "name", job.Name)
			} else {
				r.Recorder.Event(t, "Normal", "Cleanup", fmt.Sprintf("Run(%s) kubernetes job was deleted", t.Status.RunID))
			}
		}

		if t.Spec.Destroy {
			r.Recorder.Event(t, "Normal", "Destroyed", fmt.Sprintf("Run(%s) completed with terraform destroy", t.Status.RunID))
		} else {
			r.Recorder.Event(t, "Normal", "Completed", fmt.Sprintf("Run(%s) completed", t.Status.RunID))
		}

		// Always bail out after updating the status
		err := r.updateRunStatus(ctx, t, v1alpha1.RunCompleted)
		return ctrl.Result{}, err
	}

	// job failed
	if job.Status.Failed > 0 {
		r.Log.Error(errorscore.New("job failed"), "terraform run job failed to complete", "name", job.Name)

		r.Recorder.Event(t, "Warning", "Failed", fmt.Sprintf("Run(%s) failed", t.Status.RunID))

		// Always bail out after updating the status
		err = r.updateRunStatus(ctx, t, v1alpha1.RunFailed)
		return ctrl.Result{}, err
	}

	// job is still running
	return ctrl.Result{RequeueAfter: r.requeueJobWatch}, nil
}

// updateRunStatus updates the status of a Terraform run with the provided status.
// It sets the ObservedGeneration, manages timestamps for started/completed runs,
// records metrics for specific statuses, and persists the status update to the cluster.
func (r *TerraformReconciler) updateRunStatus(
	ctx context.Context, t *terraform.TerraformManipulator, status v1alpha1.TerraformRunStatus) error {

	t.Status.RunStatus = status
	t.Status.ObservedGeneration = t.Generation

	if status == v1alpha1.RunStarted {
		t.Status.StartedTime = time.Now().Format(time.UnixDate)
		t.Status.OutputSecretName = t.GetOutputSecretName().Name
	}

	// set completion time of the run only if status is completed/failed
	if status == v1alpha1.RunCompleted || status == v1alpha1.RunFailed {
		t.Status.CompletionTime = time.Now().Format(time.UnixDate)
	}

	// record the status only if completed/failed/waiting
	if status == v1alpha1.RunCompleted || status == v1alpha1.RunFailed || status == v1alpha1.RunWaitingForDependency {
		r.MetricsRecorder.RecordStatus(t.Name, t.Namespace, status)
	}

	err := r.Status().Update(ctx, t.Terraform)
	return err
}

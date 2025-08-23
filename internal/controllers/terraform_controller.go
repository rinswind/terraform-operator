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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/go-logr/logr"
	"github.com/kuptan/terraform-operator/api/v1alpha1"
	"github.com/kuptan/terraform-operator/internal/metrics"
	"github.com/kuptan/terraform-operator/internal/resources"
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
	t := &resources.TerraformManipulator{Terraform: run}

	if !controllerutil.ContainsFinalizer(t, v1alpha1.TerraformFinalizer) {
		controllerutil.AddFinalizer(t, v1alpha1.TerraformFinalizer)

		if err := r.Update(ctx, t); err != nil {
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

// SetupWithManager sets up the controller with the Manager.
// TODO: Should I configure the cache to load the objects owner by Terraform?
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

func (r *TerraformReconciler) updateRunStatus(
	ctx context.Context, t *resources.TerraformManipulator, status v1alpha1.TerraformRunStatus) error {

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

	return r.Status().Update(ctx, t)
}

func (r *TerraformReconciler) handleRunCreate(ctx context.Context, t *resources.TerraformManipulator) (ctrl.Result, error) {
	dependencies, err := r.checkDependencies(ctx, t)

	if err != nil {
		if t.IsWaiting() {
			return ctrl.Result{RequeueAfter: r.requeueDependency}, nil
		}

		r.Recorder.Event(t, "Normal", "Waiting", "Dependencies are not yet completed")

		// Always bail out after updating the status
		r.updateRunStatus(ctx, t, v1alpha1.RunWaitingForDependency)
		return ctrl.Result{RequeueAfter: r.requeueDependency}, nil
	}

	t.SetRunID()

	setVariablesFromDependencies(t, dependencies)

	_, err = r.CreateTerraformRun(ctx, t)
	if err != nil {
		r.Log.Error(err, "failed create a terraform run")

		// Always bail out after updating the status
		r.updateRunStatus(ctx, t, v1alpha1.RunFailed)
		return ctrl.Result{}, err
	}

	r.Log.Info("cleaning up old resources if exist")

	if err = r.CleanupResources(ctx, t); err != nil {
		r.Log.Error(err, "failed to cleanup resources")
	}

	// Always bail out after updating the status
	r.updateRunStatus(ctx, t, v1alpha1.RunStarted)
	return ctrl.Result{}, nil
}

func (r *TerraformReconciler) handleRunUpdate(ctx context.Context, t *resources.TerraformManipulator) (ctrl.Result, error) {

	r.Recorder.Event(t, "Normal", "Updated", "Creating a new run job")

	return r.handleRunCreate(ctx, t)
}

func (r *TerraformReconciler) handleRunDelete(ctx context.Context, t *resources.TerraformManipulator) (ctrl.Result, error) {
	r.Log.Info("terraform run is being deleted", "name", t.Name)

	r.MetricsRecorder.RecordStatus(t.Name, t.Namespace, v1alpha1.RunDeleted)
	controllerutil.RemoveFinalizer(t, v1alpha1.TerraformFinalizer)

	if err := r.Update(ctx, t.Terraform); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *TerraformReconciler) handleRunJobWatch(ctx context.Context, t *resources.TerraformManipulator) (ctrl.Result, error) {
	job, err := r.getJobForRun(ctx, t, t.Status.RunID)
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
		r.updateRunStatus(ctx, t, v1alpha1.RunRunning)
		return ctrl.Result{}, nil
	}

	// job is successful
	if job.Status.Succeeded > 0 {
		r.Log.Info("terraform run job completed successfully")

		if t.Spec.DeleteCompletedJobs {
			r.Log.Info("deleting completed job")

			if err := r.DeleteAfterCompletion(ctx, t); err != nil {
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
		r.updateRunStatus(ctx, t, v1alpha1.RunCompleted)
		return ctrl.Result{}, nil
	}

	// job failed
	if job.Status.Failed > 0 {
		r.Log.Error(errorscore.New("job failed"), "terraform run job failed to complete", "name", job.Name)

		r.Recorder.Event(t, "Warning", "Failed", fmt.Sprintf("Run(%s) failed", t.Status.RunID))

		// Always bail out after updating the status
		r.updateRunStatus(ctx, t, v1alpha1.RunFailed)
		return ctrl.Result{}, nil
	}

	// job is still running
	return ctrl.Result{RequeueAfter: r.requeueJobWatch}, nil
}

func (r *TerraformReconciler) checkDependencies(ctx context.Context, t *resources.TerraformManipulator) ([]v1alpha1.Terraform, error) {
	dependencies := []v1alpha1.Terraform{}

	for _, d := range t.Spec.DependsOn {
		if d.Namespace == "" {
			d.Namespace = t.Namespace
		}

		dName := types.NamespacedName{
			Namespace: d.Namespace,
			Name:      d.Name,
		}

		var dRun v1alpha1.Terraform

		err := r.Get(ctx, dName, &dRun)

		if err != nil {
			return dependencies, fmt.Errorf("unable to get '%s' dependency: %w", dName, err)
		}

		if dRun.Generation != dRun.Status.ObservedGeneration {
			return dependencies, fmt.Errorf("dependency '%s' is not ready", dName)
		}

		if dRun.Status.RunStatus != v1alpha1.RunCompleted {
			return dependencies, fmt.Errorf("dependency '%s' is not ready", dName)
		}

		dependencies = append(dependencies, dRun)
	}

	return dependencies, nil
}

// setVariablesFromDependencies sets the variable from the output of a dependency
// this currently only works with runs within the same namespace
func setVariablesFromDependencies(run *resources.TerraformManipulator, dependencies []v1alpha1.Terraform) {
	if len(dependencies) == 0 {
		return
	}

	for _, v := range run.Spec.Variables {
		if v.DependencyRef == nil {
			continue
		}

		for index, d := range dependencies {
			if d.Name != v.DependencyRef.Name || d.Namespace != run.Namespace {
				continue
			}

			tfVarRef := &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					Key: v.DependencyRef.Key,
					LocalObjectReference: v1.LocalObjectReference{
						Name: d.Status.OutputSecretName,
					},
				},
			}

			tfVar := v1alpha1.Variable{
				Key:           v.Key,
				DependencyRef: v.DependencyRef,
				ValueFrom:     tfVarRef,
			}

			// remove the current variable from the list
			run.Spec.Variables = append(run.Spec.Variables[:index], run.Spec.Variables[index+1:]...)
			// add a new variable with the valueFrom
			run.Spec.Variables = append(run.Spec.Variables, tfVar)
		}
	}
}

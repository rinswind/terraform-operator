package controllers

import (
	"context"

	"github.com/kuptan/terraform-operator/internal/resources"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateTerraformRun creates the Kubernetes objects to start the workflow/run
//
// (RBAC (service account & Role), ConfigMap for the terraform module file,
// Secret to store the outputs if any, will be empty if no outputs are defined,
// Job to execute the workflow/run)
func (r *TerraformReconciler) CreateTerraformRun(ctx context.Context, t *resources.TerraformManipulator) (*batchv1.Job, error) {
	if err := r.createRbacConfigIfNotExist(ctx, t); err != nil {
		return nil, err
	}

	_, err := r.createConfigMapForModule(ctx, t)
	if err != nil {
		return nil, err
	}

	_, err = r.createSecretForOutputs(ctx, t)
	if err != nil {
		return nil, err
	}

	job, err := r.createJobForRun(ctx, t)
	if err != nil {
		return nil, err
	}

	return job, nil
}

// DeleteAfterCompletion removes the Kubernetes of the workflow/run once completed
func (r *TerraformReconciler) DeleteAfterCompletion(ctx context.Context, t *resources.TerraformManipulator) error {
	return r.deleteJobByRun(ctx, t, t.Status.RunID)
}

// CleanupResources cleans up old resources (secrets & configmaps)
func (r *TerraformReconciler) CleanupResources(ctx context.Context, t *resources.TerraformManipulator) error {
	previousRunID := t.Status.PreviousRunID

	if previousRunID == "" {
		return nil
	}

	// delete the older job
	if err := r.deleteJobByRun(ctx, t, previousRunID); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	// delete the older configmap that holds the module
	if err := r.deleteConfigMapByRun(ctx, t, previousRunID); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// getJobForRun returns the Kubernetes Job of a specific workflow/run
func (r *TerraformReconciler) getJobForRun(ctx context.Context, t *resources.TerraformManipulator, runID string) (*batchv1.Job, error) {
	jobName := types.NamespacedName{
		Name:      resources.GetUniqueResourceName(t.ObjectMeta.Name, runID),
		Namespace: t.ObjectMeta.Namespace,
	}

	obj := &batchv1.Job{}

	if err := r.Get(ctx, jobName, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// createJobForRun creates a Kubernetes Job to execute the workflow/run
func (r *TerraformReconciler) createJobForRun(ctx context.Context, t *resources.TerraformManipulator) (*batchv1.Job, error) {
	job := t.GetJobSpecForRun()

	if err := r.Create(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

// deleteJobByRun deletes the Kubernetes Job of the workflow/run
func (r *TerraformReconciler) deleteJobByRun(ctx context.Context, t *resources.TerraformManipulator, runID string) error {
	job, err := r.getJobForRun(ctx, t, runID)

	if err != nil {
		return err
	}

	if job == nil {
		return nil
	}

	return r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground))
}

// createSecretForOutputs creates a secret to store the the Terraform output of the workflow/run
func (r *TerraformReconciler) createSecretForOutputs(ctx context.Context, t *resources.TerraformManipulator) (*corev1.Secret, error) {
	secretName := t.GetOutputSecretName()
	secret := &corev1.Secret{}

	err := r.Get(ctx, secretName, secret)
	if err == nil {
		return secret, nil
	}
	if !errors.IsNotFound(err) {
		return nil, err
	}

	secret = t.GetOutputSecret()
	if err := r.Create(ctx, secret); err != nil {
		return nil, err
	}

	return secret, nil
}

// createConfigMapForModule creates the ConfigMap for the Terraform workflow/run
func (r *TerraformReconciler) createConfigMapForModule(ctx context.Context, t *resources.TerraformManipulator) (*corev1.ConfigMap, error) {
	configMap, err := t.GetConfigMapSpecForModule()
	if err != nil {
		return nil, err
	}

	if err := r.Create(ctx, configMap); err != nil {
		return nil, err
	}

	return configMap, nil
}

// deleteConfigMapByRun deletes the Kubernetes Job of the workflow/run
func (r *TerraformReconciler) deleteConfigMapByRun(ctx context.Context, t *resources.TerraformManipulator, runID string) error {
	cmName := resources.GetUniqueResourceName(t.ObjectMeta.Name, runID)
	cmNamespace := t.ObjectMeta.Namespace

	configMap := &corev1.ConfigMap{}

	if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cmNamespace}, configMap); err != nil {
		return err
	}

	if err := r.Delete(ctx, configMap, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return err
	}

	return nil
}

// createRbacConfigIfNotExist validates if RBAC exist for the Terraform Runner and creates it if not exist
func (r *TerraformReconciler) createRbacConfigIfNotExist(ctx context.Context, t *resources.TerraformManipulator) error {
	saExist, err := r.isServiceAccountExist(ctx, t)
	if err != nil {
		return err
	}

	if !saExist {
		if _, err := r.createServiceAccount(ctx, t); err != nil {
			return err
		}
	}

	roleBindingExist, err := r.isRoleBindingExist(ctx, t)
	if err != nil {
		return err
	}

	if !roleBindingExist {
		if _, err := r.createRoleBinding(ctx, t); err != nil {
			return err
		}
	}

	return nil
}

// isServiceAccountExist checks whether the ServiceAccount for the Terraform Runner exist
func (r *TerraformReconciler) isServiceAccountExist(ctx context.Context, t *resources.TerraformManipulator) (bool, error) {
	err := r.Get(ctx, t.GetServiceAccountName(), &corev1.ServiceAccount{})

	if err == nil {
		return true, nil
	}

	if errors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

// isRoleBindingExist checks if the RoleBinding for the Terraform Runner exists
func (r *TerraformReconciler) isRoleBindingExist(ctx context.Context, t *resources.TerraformManipulator) (bool, error) {
	err := r.Get(ctx, t.GetRoleBindingName(), &rbacv1.RoleBinding{})

	if err == nil {
		return true, nil
	}

	if errors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

// createServiceAccount creates a Kubernetes ServiceAccount for the Terraform Runner
func (r *TerraformReconciler) createServiceAccount(ctx context.Context, t *resources.TerraformManipulator) (*corev1.ServiceAccount, error) {
	obj := t.GetServiceAccount()

	if err := r.Create(ctx, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// createRoleBinding creates a Kubernetes RoleBinding for the Terraform Runner
func (r *TerraformReconciler) createRoleBinding(ctx context.Context, t *resources.TerraformManipulator) (*rbacv1.RoleBinding, error) {
	obj := t.GetRoleBinding()

	if err := r.Create(ctx, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

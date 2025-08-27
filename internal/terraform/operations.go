package terraform

import (
	"context"

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
func (t *TerraformManipulator) CreateTerraformRun(ctx context.Context, c client.Client) (*batchv1.Job, error) {
	t.setRunID()

	if err := t.createRbacConfigIfNotExist(ctx, c); err != nil {
		return nil, err
	}

	_, err := t.createConfigMapForModule(ctx, c)
	if err != nil {
		return nil, err
	}

	_, err = t.createSecretForOutputs(ctx, c)
	if err != nil {
		return nil, err
	}

	job, err := t.createJobForRun(ctx, c)
	if err != nil {
		return nil, err
	}

	return job, nil
}

// DeleteAfterCompletion removes the Kubernetes of the workflow/run once completed
func (t *TerraformManipulator) DeleteAfterCompletion(ctx context.Context, c client.Client) error {
	return t.deleteJobByRun(ctx, c, t.Status.RunID)
}

// CleanupResources cleans up old resources (secrets & configmaps)
func (t *TerraformManipulator) CleanupResources(ctx context.Context, c client.Client) error {
	previousRunID := t.Status.PreviousRunID

	if previousRunID == "" {
		return nil
	}

	// delete the older job
	if err := t.deleteJobByRun(ctx, c, previousRunID); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	// delete the older configmap that holds the module
	if err := t.deleteConfigMapByRun(ctx, c, previousRunID); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// getJobForRun returns the Kubernetes Job of a specific workflow/run
func (t *TerraformManipulator) GetJobForRun(ctx context.Context, c client.Client, runID string) (*batchv1.Job, error) {
	jobName := types.NamespacedName{
		Name:      getUniqueResourceName(t.ObjectMeta.Name, runID),
		Namespace: t.ObjectMeta.Namespace,
	}

	obj := &batchv1.Job{}

	if err := c.Get(ctx, jobName, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// createJobForRun creates a Kubernetes Job to execute the workflow/run
func (t *TerraformManipulator) createJobForRun(ctx context.Context, c client.Client) (*batchv1.Job, error) {
	job := t.GetJobSpecForRun()

	if err := c.Create(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

// deleteJobByRun deletes the Kubernetes Job of the workflow/run
func (t *TerraformManipulator) deleteJobByRun(ctx context.Context, c client.Client, runID string) error {
	job, err := t.GetJobForRun(ctx, c, runID)

	if err != nil {
		return err
	}

	if job == nil {
		return nil
	}

	return c.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground))
}

// createSecretForOutputs creates a secret to store the the Terraform output of the workflow/run
func (t *TerraformManipulator) createSecretForOutputs(ctx context.Context, c client.Client) (*corev1.Secret, error) {
	secretName := t.GetOutputSecretName()
	secret := &corev1.Secret{}

	err := c.Get(ctx, secretName, secret)
	if err == nil {
		return secret, nil
	}
	if !errors.IsNotFound(err) {
		return nil, err
	}

	secret = t.GetOutputSecret()
	if err := c.Create(ctx, secret); err != nil {
		return nil, err
	}

	return secret, nil
}

// createConfigMapForModule creates the ConfigMap for the Terraform workflow/run
func (t *TerraformManipulator) createConfigMapForModule(ctx context.Context, c client.Client) (*corev1.ConfigMap, error) {
	configMap, err := t.GetConfigMapSpecForModule()
	if err != nil {
		return nil, err
	}

	if err := c.Create(ctx, configMap); err != nil {
		return nil, err
	}

	return configMap, nil
}

// deleteConfigMapByRun deletes the Kubernetes Job of the workflow/run
func (t *TerraformManipulator) deleteConfigMapByRun(ctx context.Context, c client.Client, runID string) error {
	cmName := getUniqueResourceName(t.ObjectMeta.Name, runID)
	cmNamespace := t.ObjectMeta.Namespace

	configMap := &corev1.ConfigMap{}

	if err := c.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cmNamespace}, configMap); err != nil {
		return err
	}

	if err := c.Delete(ctx, configMap, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return err
	}

	return nil
}

// createRbacConfigIfNotExist validates if RBAC exist for the Terraform Runner and creates it if not exist
func (t *TerraformManipulator) createRbacConfigIfNotExist(ctx context.Context, c client.Client) error {
	saExist, err := t.isServiceAccountExist(ctx, c)
	if err != nil {
		return err
	}

	if !saExist {
		if _, err := t.createServiceAccount(ctx, c); err != nil {
			return err
		}
	}

	roleBindingExist, err := t.isRoleBindingExist(ctx, c)
	if err != nil {
		return err
	}

	if !roleBindingExist {
		if _, err := t.createRoleBinding(ctx, c); err != nil {
			return err
		}
	}

	return nil
}

// isServiceAccountExist checks whether the ServiceAccount for the Terraform Runner exist
func (t *TerraformManipulator) isServiceAccountExist(ctx context.Context, c client.Client) (bool, error) {
	err := c.Get(ctx, t.GetServiceAccountName(), &corev1.ServiceAccount{})

	if err == nil {
		return true, nil
	}

	if errors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

// isRoleBindingExist checks if the RoleBinding for the Terraform Runner exists
func (t *TerraformManipulator) isRoleBindingExist(ctx context.Context, c client.Client) (bool, error) {
	err := c.Get(ctx, t.GetRoleBindingName(), &rbacv1.RoleBinding{})

	if err == nil {
		return true, nil
	}

	if errors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

// createServiceAccount creates a Kubernetes ServiceAccount for the Terraform Runner
func (t *TerraformManipulator) createServiceAccount(ctx context.Context, c client.Client) (*corev1.ServiceAccount, error) {
	obj := t.GetServiceAccount()

	if err := c.Create(ctx, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// createRoleBinding creates a Kubernetes RoleBinding for the Terraform Runner
func (t *TerraformManipulator) createRoleBinding(ctx context.Context, c client.Client) (*rbacv1.RoleBinding, error) {
	obj := t.GetRoleBinding()

	if err := c.Create(ctx, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

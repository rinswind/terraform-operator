package v1alpha1

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type clientKey = struct{}

var ClientKey = clientKey{}

func getClient(ctx context.Context) client.Client {
	val := ctx.Value(ClientKey)
	client, ok := val.(client.Client)
	if !ok {
		// TODO Panic instead?
		return nil
	}
	return client
}

// IsSubmitted evaluates if the workflow/run is created for the first time
func (t *Terraform) IsSubmitted() bool {
	return t.Status.RunID == ""
}

// IsStarted evaluates that the workflow/run is started
func (t *Terraform) IsStarted() bool {
	allowedStatuses := map[TerraformRunStatus]bool{
		RunStarted: true,
		RunRunning: true,
	}

	return allowedStatuses[t.Status.RunStatus]
}

// IsRunning evaluates that the workflow/run is running
func (t *Terraform) IsRunning() bool {
	return t.Status.RunStatus == RunRunning
}

// IsUpdated evaluates if the workflow/run was updated
func (t *Terraform) IsUpdated() bool {
	return t.Generation > 0 && t.Generation > t.Status.ObservedGeneration
}

// IsWaiting evaluates if the workflow/run is waiting for a dependency
func (t *Terraform) IsWaiting() bool {
	return t.Status.RunStatus == RunWaitingForDependency
}

// HasErrored evaluates if the workflow/run failed
func (t *Terraform) HasErrored() bool {
	return t.Status.RunStatus == RunFailed
}

// SetRunID sets a new value for the run ID
func (t *Terraform) SetRunID() {
	if t.Status.RunID != "" {
		t.Status.PreviousRunID = t.Status.RunID
	}
	t.Status.RunID = random(6)
}

// GetOwnerReference returns the Kubernetes owner reference meta
func (t *Terraform) GetOwnerReference() metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: fmt.Sprintf("%s/%s", GroupVersion.Group, GroupVersion.Version),
		Kind:       t.Kind,
		Name:       t.Name,
		UID:        t.GetUID(),
	}
}

// setBackendCfgIfNotExist sets the default backend to Kunernetes if not provided
func setBackendCfgIfNotExist(run *Terraform) {
	if run.Spec.Backend == "" {
		run.Spec.Backend = fmt.Sprintf(`backend "kubernetes" {
  secret_suffix     = "%s"
  in_cluster_config = true
  namespace         = "%s"
}
`, run.ObjectMeta.Name, run.ObjectMeta.Namespace)
	}
}

// runnerRBACName is the RBAC name that will be used in the role and service account creation
// if they're not found
const runnerRBACName string = "terraform-runner"

// CreateTerraformRun creates the Kubernetes objects to start the workflow/run
//
// (RBAC (service account & Role), ConfigMap for the terraform module file,
// Secret to store the outputs if any, will be empty if no outputs are defined,
// Job to execute the workflow/run)
func (t *Terraform) CreateTerraformRun(ctx context.Context, namespacedName types.NamespacedName) (*batchv1.Job, error) {
	setBackendCfgIfNotExist(t)

	if err := createRbacConfigIfNotExist(ctx, runnerRBACName, namespacedName.Namespace); err != nil {
		return nil, err
	}

	_, err := createConfigMapForModule(ctx, namespacedName, t)
	if err != nil {
		return nil, err
	}

	_, err = createSecretForOutputs(ctx, namespacedName, t)
	if err != nil {
		return nil, err
	}

	job, err := createJobForRun(ctx, t)
	if err != nil {
		return nil, err
	}

	return job, nil
}

// DeleteAfterCompletion removes the Kubernetes of the workflow/run once completed
func (t *Terraform) DeleteAfterCompletion(ctx context.Context) error {
	if err := deleteJobByRun(ctx, t.Name, t.Namespace, t.Status.RunID); err != nil {
		return err
	}

	return nil
}

// GetOutputSecretName returns the secret name of the Terraform outputs
func (t *Terraform) GetOutputSecretName() string {
	return getOutputSecretname(t.Name)
}

// CleanupResources cleans up old resources (secrets & configmaps)
func (t *Terraform) CleanupResources(ctx context.Context) error {
	previousRunID := t.Status.PreviousRunID

	if previousRunID == "" {
		return nil
	}

	// delete the older job
	if err := deleteJobByRun(ctx, t.Name, t.Namespace, previousRunID); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	// delete the older configmap that holds the module
	if err := deleteConfigMapByRun(ctx, t.Name, t.Namespace, previousRunID); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// GetJobByRun returns the Kubernetes job of the workflow/run
func (t *Terraform) GetJobByRun(ctx context.Context) (*batchv1.Job, error) {
	job, err := getJobForRun(ctx, t.Name, t.Namespace, t.Status.RunID)

	if err != nil {
		return nil, err
	}

	return job, err
}

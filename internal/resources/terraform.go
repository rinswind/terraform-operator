package resources

import (
	"fmt"

	"github.com/kuptan/terraform-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TerraformManipulator struct {
	*v1alpha1.Terraform
}

// IsSubmitted evaluates if the workflow/run is created for the first time
func (t *TerraformManipulator) IsSubmitted() bool {
	return t.Status.RunID == ""
}

// IsStarted evaluates that the workflow/run is started
func (t *TerraformManipulator) IsStarted() bool {
	allowedStatuses := map[v1alpha1.TerraformRunStatus]bool{
		v1alpha1.RunStarted: true,
		v1alpha1.RunRunning: true,
	}

	return allowedStatuses[t.Status.RunStatus]
}

// IsRunning evaluates that the workflow/run is running
func (t *TerraformManipulator) IsRunning() bool {
	return t.Status.RunStatus == v1alpha1.RunRunning
}

// IsUpdated evaluates if the workflow/run was updated
func (t *TerraformManipulator) IsUpdated() bool {
	return t.Generation > 0 && t.Generation > t.Status.ObservedGeneration
}

// IsWaiting evaluates if the workflow/run is waiting for a dependency
func (t *TerraformManipulator) IsWaiting() bool {
	return t.Status.RunStatus == v1alpha1.RunWaitingForDependency
}

// HasErrored evaluates if the workflow/run failed
func (t *TerraformManipulator) HasErrored() bool {
	return t.Status.RunStatus == v1alpha1.RunFailed
}

// SetRunID sets a new value for the run ID
func (t *TerraformManipulator) SetRunID() {
	if t.Status.RunID != "" {
		t.Status.PreviousRunID = t.Status.RunID
	}
	t.Status.RunID = random(6)
}

// GetOwnerReference returns the Kubernetes owner reference meta
func (t *TerraformManipulator) GetOwnerReference() metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: fmt.Sprintf("%s/%s", v1alpha1.GroupVersion.Group, v1alpha1.GroupVersion.Version),
		Kind:       t.Kind,
		Name:       t.Name,
		UID:        t.GetUID(),
	}
}

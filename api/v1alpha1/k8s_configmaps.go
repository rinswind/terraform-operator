package v1alpha1

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getConfigMapSpecForModule returns a Kubernetes ConifgMap spec for the terraform module
// This configmap will be mounted in the Terraform Runner pod
func getConfigMapSpecForModule(
	name string, namespace string, module string, runID string, owner metav1.OwnerReference) *corev1.ConfigMap {

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getUniqueResourceName(name, runID),
			Namespace: namespace,
			Labels:    getCommonLabels(name, runID),
			OwnerReferences: []metav1.OwnerReference{
				owner,
			},
		},
		Data: map[string]string{
			"main.tf": module,
		},
	}

	return cm
}

// createConfigMapForModule creates the ConfigMap for the Terraform workflow/run
func createConfigMapForModule(
	ctx context.Context, namespacedName types.NamespacedName, run *Terraform) (*corev1.ConfigMap, error) {

	tpl, err := getTerraformModuleFromTemplate(run)
	if err != nil {
		return nil, err
	}

	configMap := getConfigMapSpecForModule(
		namespacedName.Name,
		namespacedName.Namespace,
		string(tpl), run.Status.RunID,
		run.GetOwnerReference())

	if err := getClient(ctx).Create(ctx, configMap); err != nil {
		return nil, err
	}

	return configMap, nil
}

// deleteConfigMapByRun deletes the Kubernetes Job of the workflow/run
func deleteConfigMapByRun(ctx context.Context, runName string, namespace string, runID string) error {
	configMapName := getUniqueResourceName(runName, runID)

	configMap := &corev1.ConfigMap{}

	if err := getClient(ctx).Get(ctx, types.NamespacedName{Name: configMapName, Namespace: namespace}, configMap); err != nil {
		return err
	}

	if err := getClient(ctx).Delete(ctx, configMap, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return err
	}

	return nil
}

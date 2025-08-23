package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getConfigMapSpecForModule returns a Kubernetes ConifgMap spec for the terraform module
// This configmap will be mounted in the Terraform Runner pod
func (t *TerraformManipulator) GetConfigMapSpecForModule() (*corev1.ConfigMap, error) {
	tpl, err := t.GetTerraformModuleFromTemplate()
	if err != nil {
		return nil, err
	}

	name := t.ObjectMeta.Name
	namespace := t.ObjectMeta.Namespace
	runID := t.Status.RunID

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetUniqueResourceName(name, runID),
			Namespace: namespace,
			Labels:    getCommonLabels(name, runID),
			OwnerReferences: []metav1.OwnerReference{
				t.GetOwnerReference(),
			},
		},
		Data: map[string]string{
			"main.tf": string(tpl),
		},
	}

	return cm, nil
}

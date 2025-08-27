package terraform

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (t *TerraformManipulator) GetOutputSecretName() types.NamespacedName {
	return types.NamespacedName{Name: getOutputSecretName(t.Name), Namespace: t.Namespace}
}

func (t *TerraformManipulator) GetOutputSecret() *corev1.Secret {
	name := t.GetOutputSecretName()

	obj := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			Labels:    getCommonLabels(t.ObjectMeta.Name, t.Status.RunID),
			OwnerReferences: []metav1.OwnerReference{
				t.getOwnerReference(),
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}

	return obj
}

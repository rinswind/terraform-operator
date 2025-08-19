package v1alpha1

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// createSecretForOutputs creates a secret to store the the Terraform output of the workflow/run
func createSecretForOutputs(ctx context.Context, runName types.NamespacedName, t *Terraform) (*corev1.Secret, error) {
	secretName := types.NamespacedName{Name: getOutputSecretname(runName.Name), Namespace: runName.Namespace}

	secret := &corev1.Secret{}

	err := getClient(ctx).Get(ctx, secretName, secret)

	if err == nil {
		return secret, nil
	}

	if !errors.IsNotFound(err) {
		return nil, err
	}

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName.Name,
			Namespace: secretName.Namespace,
			Labels:    getCommonLabels(runName.Name, t.Status.RunID),
			OwnerReferences: []metav1.OwnerReference{
				t.GetOwnerReference(),
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}

	if err := getClient(ctx).Create(ctx, secret); err != nil {
		return nil, err
	}

	return secret, nil
}

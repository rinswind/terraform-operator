package v1alpha1

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// createServiceAccount creates a Kubernetes ServiceAccount for the Terraform Runner
func createServiceAccount(ctx context.Context, name string, namespace string) (*corev1.ServiceAccount, error) {
	obj := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if err := getClient(ctx).Create(ctx, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// createRoleBinding creates a Kubernetes RoleBinding for the Terraform Runner
func createRoleBinding(ctx context.Context, name string, namespace string) (*rbacv1.RoleBinding, error) {
	obj := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     name,
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      name,
				Namespace: namespace,
			},
		},
	}

	if err := getClient(ctx).Create(ctx, obj); err != nil {
		return nil, err
	}

	return obj, nil
}

// isServiceAccountExist checks whether the ServiceAccount for the Terraform Runner exist
func isServiceAccountExist(ctx context.Context, name string, namespace string) (bool, error) {
	err := getClient(ctx).Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &corev1.ServiceAccount{})

	if err == nil {
		return true, nil
	}

	if errors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

// isRoleBindingExist checks if the RoleBinding for the Terraform Runner exists
func isRoleBindingExist(ctx context.Context, name string, namespace string) (bool, error) {
	err := getClient(ctx).Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &rbacv1.RoleBinding{})

	if err == nil {
		return true, nil
	}

	if errors.IsNotFound(err) {
		return false, nil
	}

	return false, err
}

// createRbacConfigIfNotExist validates if RBAC exist for the Terraform Runner and creates it if not exist
func createRbacConfigIfNotExist(ctx context.Context, name string, namespace string) error {
	saExist, err := isServiceAccountExist(ctx, name, namespace)
	if err != nil {
		return err
	}

	roleBindingExist, err := isRoleBindingExist(ctx, name, namespace)
	if err != nil {
		return err
	}

	if !saExist {
		if _, err := createServiceAccount(ctx, name, namespace); err != nil {
			return err
		}
	}

	if !roleBindingExist {
		if _, err := createRoleBinding(ctx, name, namespace); err != nil {
			return err
		}
	}

	return nil
}

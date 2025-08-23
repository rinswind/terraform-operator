package resources

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// runnerRBACName is the RBAC name that will be used in the role and service account creation
	// if they're not found
	runnerRBACName string = "terraform-runner"
)

func (t *TerraformManipulator) GetServiceAccountName() types.NamespacedName {
	return types.NamespacedName{Name: runnerRBACName, Namespace: t.ObjectMeta.Namespace}
}

func (t *TerraformManipulator) GetServiceAccount() *corev1.ServiceAccount {
	name := t.GetServiceAccountName()

	obj := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
	}

	return obj
}

func (t *TerraformManipulator) GetRoleBindingName() types.NamespacedName {
	return types.NamespacedName{Name: runnerRBACName, Namespace: t.ObjectMeta.Namespace}
}

func (t *TerraformManipulator) GetRoleBinding() *rbacv1.RoleBinding {
	name := t.GetRoleBindingName()

	obj := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     runnerRBACName,
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      runnerRBACName,
				Namespace: t.ObjectMeta.Namespace,
			},
		},
	}

	return obj
}

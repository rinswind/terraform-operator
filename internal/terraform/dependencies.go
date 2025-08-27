package terraform

import (
	"context"
	"fmt"

	"github.com/kuptan/terraform-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *TerraformManipulator) CheckDependencies(ctx context.Context, c client.Client) ([]TerraformManipulator, error) {
	dependencies := []TerraformManipulator{}

	for _, d := range t.Spec.DependsOn {
		if d.Namespace == "" {
			d.Namespace = t.Namespace
		}

		depName := types.NamespacedName{Namespace: d.Namespace, Name: d.Name}

		dep := &v1alpha1.Terraform{}

		err := c.Get(ctx, depName, dep)

		if err != nil {
			return dependencies, fmt.Errorf("unable to get '%s' dependency: %w", depName, err)
		}

		if dep.Generation != dep.Status.ObservedGeneration {
			return dependencies, fmt.Errorf("dependency '%s' is not ready", depName)
		}

		if dep.Status.RunStatus != v1alpha1.RunCompleted {
			return dependencies, fmt.Errorf("dependency '%s' is not ready", depName)
		}

		dependencies = append(dependencies, TerraformManipulator{Terraform: dep})
	}

	return dependencies, nil
}

// SetVariablesFromDependencies sets the variable from the output of a dependency
// this currently only works with runs within the same namespace
func (t *TerraformManipulator) SetVariablesFromDependencies(dependencies []TerraformManipulator) {
	if len(dependencies) == 0 {
		return
	}

	for _, v := range t.Spec.Variables {
		if v.DependencyRef == nil {
			continue
		}

		for index, dep := range dependencies {
			if dep.Name != v.DependencyRef.Name || dep.Namespace != t.Namespace {
				continue
			}

			tfVarRef := &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					Key: v.DependencyRef.Key,
					LocalObjectReference: v1.LocalObjectReference{
						Name: dep.Status.OutputSecretName,
					},
				},
			}

			tfVar := v1alpha1.Variable{
				Key:           v.Key,
				DependencyRef: v.DependencyRef,
				ValueFrom:     tfVarRef,
			}

			// remove the current variable from the list
			t.Spec.Variables = append(t.Spec.Variables[:index], t.Spec.Variables[index+1:]...)
			// add a new variable with the valueFrom
			t.Spec.Variables = append(t.Spec.Variables, tfVar)
		}
	}
}

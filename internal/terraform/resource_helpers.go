package terraform

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// getVolumeSpec returns a volume spec
func getVolumeSpec(name string, source corev1.VolumeSource) corev1.Volume {
	return corev1.Volume{
		Name:         name,
		VolumeSource: source,
	}
}

// getVolumeSpecFromConfigMap returns a volume spec from configMap
func getVolumeSpecFromConfigMap(volumeName string, configMapName string) corev1.Volume {
	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}
}

// getEmptyDirVolume returns and emptyDir volume spec
func getEmptyDirVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// getVolumeMountSpecWithSubPath returns a volume mount spec with subpath option
func getVolumeMountSpecWithSubPath(volumeName string, mountPath string, subPath string, readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		ReadOnly:  readOnly,
		SubPath:   subPath,
	}
}

// getVolumeMountSpec returns a volume mount spec
func getVolumeMountSpec(volumeName string, mountPath string, readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		ReadOnly:  readOnly,
	}
}

// getEnvVariable returns a Kubernetes environment variable spec
func getEnvVariable(name string, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}

// getEnvVariableFromFieldSelector returns a Kubernetes environment variable from a field selector
func getEnvVariableFromFieldSelector(name string, path string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: path,
			},
		},
	}
}

// returns common labels to be attached to children resources
func getCommonLabels(name string, runID string) map[string]string {
	return map[string]string{
		"terraformRunName": name,
		"terraformRunId":   runID,
		"component":        "Terraform-run",
		"owner":            "run.terraform-operator.io",
	}
}

// GetUniqueResourceName returns a unique name for the terraform Run job
func getUniqueResourceName(name string, runID string) string {
	return fmt.Sprintf("%s-%s", truncateResourceName(name, 220), runID)
}

// GetOutputSecretName returns a unique name for the terraform Run job
func getOutputSecretName(runName string) string {
	return fmt.Sprintf("%s-outputs", truncateResourceName(runName, 220))
}

// generates a random alphanumeric based on the length provided
func random(n int64) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz123456790")

	b := make([]rune, n)

	for i := range b {
		generated, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))

		b[i] = letters[generated.Int64()]
	}
	return string(b)
}

func truncateResourceName(name string, max int) string {
	if len(name) < max {
		return name
	}

	name = name[0:max]

	// End in alphanum, Assume only "-" and "." can be in name
	name = strings.TrimRight(name, "-")
	name = strings.TrimRight(name, ".")

	return name
}

// returns a bool on whether a string is available in a given array of string
// func containsString(slice []string, s string) bool {
// 	for _, item := range slice {
// 		if item == s {
// 			return true
// 		}
// 	}
// 	return false
// }

// // removes a string from a given array of string
// func removeString(slice []string, s string) (result []string) {
// 	for _, item := range slice {
// 		if item == s {
// 			continue
// 		}
// 		result = append(result, item)
// 	}
// 	return
// }

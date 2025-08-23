package resources

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kuptan/terraform-operator/api/v1alpha1"
	"github.com/kuptan/terraform-operator/internal/kube"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kubernetes Jobs", func() {
	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	Context("Job Spec Validation", func() {
		var job *batchv1.Job

		trop := &TerraformManipulator{&v1alpha1.Terraform{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bar",
				Namespace: "default",
			},
			Spec: v1alpha1.TerraformSpec{
				TerraformVersion: "1.0.2",
				Workspace:        "dev",
				Module: v1alpha1.Module{
					Source:  "IbraheemAlSaady/test/module",
					Version: "0.0.1",
				},
				GitSSHKey: &v1alpha1.GitSSHKey{
					ValueFrom: &corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "mysecret",
						},
					},
				},
				Destroy:             false,
				DeleteCompletedJobs: false,
			},
			Status: v1alpha1.TerraformStatus{
				RunID: "12345",
			},
		}}

		ownerRef := metav1.OwnerReference{
			APIVersion: fmt.Sprintf("%s/%s", v1alpha1.GroupVersion.Group, v1alpha1.GroupVersion.Version),
			Kind:       "terraform",
			Name:       "foot",
			UID:        "1234",
		}

		It("returns the job spec and should not be null", func() {
			jobSpec := trop.getJobSpecForRun(ownerRef)

			Expect(jobSpec).ToNot(BeNil())

			job = jobSpec
		})

		It("should contain a volume for the git ssh", func() {
			var sshVolume *corev1.Volume

			for _, v := range job.Spec.Template.Spec.Volumes {
				vol := v
				if v.Name == gitSSHKeyVolumeName {
					sshVolume = &vol
					break
				}
			}

			Expect(sshVolume).ToNot(BeNil())
			Expect(sshVolume.Name).To(Equal(gitSSHKeyVolumeName))
			Expect(sshVolume.VolumeSource.Secret.SecretName).To(Equal(trop.Spec.GitSSHKey.ValueFrom.Secret.SecretName))
		})

		It("should contain an environment variable for Terraform workspace", func() {
			var envVar corev1.EnvVar

			for _, e := range job.Spec.Template.Spec.Containers[0].Env {
				if e.Name == "TERRAFORM_WORKSPACE" {
					envVar = e
					break
				}
			}

			Expect(envVar).ToNot(BeNil())
			Expect(envVar.Value).To(Equal(trop.Spec.Workspace))
		})
	})

	Context("Multi var file job", func() {
		var job *batchv1.Job

		trop := &TerraformManipulator{&v1alpha1.Terraform{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bar1",
				Namespace: "default",
			},
			Spec: v1alpha1.TerraformSpec{
				TerraformVersion: "1.0.2",
				Module: v1alpha1.Module{
					Source:  "IbraheemAlSaady/test/module",
					Version: "0.0.1",
				},
				VariableFiles: []v1alpha1.VariableFile{
					{
						Key: "common",
						ValueFrom: &corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "cfg1",
								},
							},
						},
					},
					{
						Key: "data",
						ValueFrom: &corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "cfg2",
								},
							},
						},
					},
				},
			},
			Status: v1alpha1.TerraformStatus{
				RunID: "12345",
			},
		}}

		ownerRef := metav1.OwnerReference{
			APIVersion: fmt.Sprintf("%s/%s", v1alpha1.GroupVersion.Group, v1alpha1.GroupVersion.Version),
			Kind:       "terraform",
			Name:       "foot",
			UID:        "1234",
		}

		It("should return the job spec", func() {
			jobSpec := trop.getJobSpecForRun(ownerRef)

			Expect(jobSpec).ToNot(BeNil())

			job = jobSpec
		})

		It("should be able to create the job", func() {
			created, err := kube.ClientSet.BatchV1().Jobs("default").Create(context.Background(), job, metav1.CreateOptions{})

			Expect(err).ToNot(HaveOccurred())
			Expect(created).ToNot(BeNil())
		})
	})
})

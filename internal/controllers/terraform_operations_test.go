package controllers

import (
	"context"
	"errors"

	"github.com/kuptan/terraform-operator/api/v1alpha1"
	"github.com/kuptan/terraform-operator/internal/kube"
	"github.com/kuptan/terraform-operator/internal/resources"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakebatchv1 "k8s.io/client-go/kubernetes/typed/batch/v1/fake"
	fakecorev1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	fakerbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1/fake"
	"k8s.io/client-go/testing"
)

var _ = Describe("Terraform", func() {
	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
		resetClientSet()
	})

	Context("Terraform Kubernetes Create/Get/Delete", func() {
		var created, fetched *TerraformManipulator

		key := types.NamespacedName{
			Name:      "crud",
			Namespace: "default",
		}

		It("should handle create/get/delete for a Terraform Object", func() {
			created = &TerraformManipulator{
				Terraform: &v1alpha1.Terraform{
					ObjectMeta: metav1.ObjectMeta{
						Name:      key.Name,
						Namespace: key.Namespace,
					},
					Spec: v1alpha1.TerraformSpec{
						TerraformVersion: "1.0.2",
						Module: v1alpha1.Module{
							Source:  "IbraheemAlSaady/test/module",
							Version: "0.0.1",
						},
						Variables: []v1alpha1.Variable{
							{
								Key:   "length",
								Value: "16",
							},
						},
						Destroy:             false,
						DeleteCompletedJobs: false,
					},
				}}

			By("creating a terraform run")
			Expect(k8sClient.Create(context.TODO(), created)).To(Succeed())

			fetched = &TerraformManipulator{}
			Expect(k8sClient.Get(context.TODO(), key, fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			By("deleting the created object")
			Expect(k8sClient.Delete(context.TODO(), created)).To(Succeed())
			Expect(k8sClient.Get(context.TODO(), key, created)).ToNot(Succeed())
		})
	})

	Context("Terraform Statuses", func() {
		It("should correctly handle run statuses", func() {
			run1 := &v1alpha1.Terraform{
				Status: v1alpha1.TerraformStatus{
					RunID: "",
				},
			}
			t1 := &TerraformManipulator{client: nil, Terraform: run1}

			By("run was just submitted")
			Expect(t1.IsSubmitted()).To(BeTrue())

			run2 := &v1alpha1.Terraform{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: v1alpha1.TerraformStatus{
					RunStatus: v1alpha1.RunStarted,
				},
			}

			t2 := &TerraformManipulator{client: nil, Terraform: run2}
			t2.SetRunID()

			Expect(run2.Status.RunID).ToNot(BeEmpty())

			By("run is now in a Started state")
			Expect(t2.IsSubmitted()).To(BeFalse())
			Expect(t2.IsStarted()).To(BeTrue())

			run2.Status.RunStatus = v1alpha1.RunRunning
			By("run is now in a Running state")
			Expect(t2.IsStarted()).To(BeTrue())
			Expect(t2.IsRunning()).To(BeTrue())

			run2.Status.RunStatus = v1alpha1.RunFailed
			By("run is now in a Failed state")
			Expect(t2.HasErrored()).To(BeTrue())

			run2.Status.ObservedGeneration = run2.Generation
			run2.Generation = 2

			By("run generation was updated")
			Expect(t2.IsUpdated()).To(BeTrue())

			run2.Status.RunStatus = v1alpha1.RunWaitingForDependency
			By("run is now in a waiting state")
			Expect(t2.IsWaiting()).To(BeTrue())

			run2.Status.RunStatus = v1alpha1.RunCompleted
			By("run is now in a Completed state")
			Expect(t2.IsStarted()).To(BeFalse())
		})
	})

	Context("Terraform Workflow", func() {
		key := types.NamespacedName{
			Name:      "foo",
			Namespace: "default",
		}

		run := &v1alpha1.Terraform{
			ObjectMeta: metav1.ObjectMeta{
				Name:      key.Name,
				Namespace: key.Namespace,
			},
			Spec: v1alpha1.TerraformSpec{
				TerraformVersion: "1.0.2",
				Module: v1alpha1.Module{
					Source:  "IbraheemAlSaady/test/module",
					Version: "0.0.1",
				},
				Variables: []v1alpha1.Variable{
					v1alpha1.Variable{
						Key:   "length",
						Value: "16",
					},
				},
				Destroy:             false,
				DeleteCompletedJobs: false,
			},
		}

		t := TerraformManipulator{client: nil, Terraform: run}

		It("should handle a terraform run job", func() {
			run.Status.RunID = "1234"

			job, err := t.CreateTerraformRun(context.Background(), key)
			Expect(err).ToNot(HaveOccurred(), "failed to create a terraform run")
			Expect(job.Name).ToNot(BeEmpty())

			job, err = t.GetJobByRun(context.Background())

			Expect(err).ToNot(HaveOccurred(), "run job was not found")
			Expect(job.Name).ToNot(BeEmpty())

			err = t.DeleteAfterCompletion(context.Background())

			Expect(err).ToNot(HaveOccurred(), "failed to clean up resources")
		})

		It("should get the owner preference", func() {
			owner := t.GetOwnerReference()

			Expect(owner).ToNot(BeNil())
		})

		It("should handle resource cleanup when there is no previous run", func() {
			run := &v1alpha1.Terraform{
				Status: v1alpha1.TerraformStatus{
					RunID:         "",
					PreviousRunID: "",
				},
			}

			Expect(run.CleanupResources(context.Background())).ToNot(HaveOccurred())
		})

		It("should handle resource cleanup when previous run exist", func() {
			run := &v1alpha1.Terraform{
				Status: v1alpha1.TerraformStatus{
					RunID:         "1234",
					PreviousRunID: "612faw",
				},
			}

			Expect(run.CleanupResources(context.Background())).ToNot(HaveOccurred())
		})

		It("should create a job when RBAC objects already exist", func() {
			key := types.NamespacedName{
				Name:      "bar",
				Namespace: "default",
			}

			run2 := &v1alpha1.Terraform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: v1alpha1.TerraformSpec{
					TerraformVersion: "1.0.2",
					Module: v1alpha1.Module{
						Source:  "IbraheemAlSaady/test/module",
						Version: "0.0.1",
					},
					Variables: []v1alpha1.Variable{
						{
							Key:   "length",
							Value: "16",
						},
					},
					Destroy:             false,
					DeleteCompletedJobs: false,
				},
				Status: v1alpha1.TerraformStatus{
					RunID: "dahwe12",
				},
			}

			err := createRbacConfigIfNotExist(context.Background(), runnerRBACName, key.Namespace)
			Expect(err).ToNot(HaveOccurred())

			job, err := run2.CreateTerraformRun(context.Background(), key)
			Expect(err).ToNot(HaveOccurred(), "failed to create a terraform run")
			Expect(job.Name).ToNot(BeEmpty())
		})
	})

	Context("Terraform Error Handling", func() {
		key := types.NamespacedName{
			Name:      "barbar",
			Namespace: "default",
		}

		run := &v1alpha1.Terraform{
			ObjectMeta: metav1.ObjectMeta{
				Name:      key.Name,
				Namespace: key.Namespace,
			},
			Spec: v1alpha1.TerraformSpec{
				TerraformVersion: "1.0.2",
				Module: v1alpha1.Module{
					Source:  "IbraheemAlSaady/test/module",
					Version: "0.0.1",
				},
				Variables: []v1alpha1.Variable{
					v1alpha1.Variable{
						Key:   "length",
						Value: "16",
					},
				},
				Destroy:             false,
				DeleteCompletedJobs: false,
			},
			Status: v1alpha1.TerraformStatus{
				RunID: "1234",
			},
		}

		name := resources.GetUniqueResourceName(run.Name, run.Status.RunID)

		It("should fail to create a run due to existing configmap", func() {
			cfg := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "default",
				},
				Data: make(map[string]string),
			}

			kube.ClientSet.CoreV1().ConfigMaps("default").Create(context.Background(), &cfg, metav1.CreateOptions{})

			job, err := run.CreateTerraformRun(context.Background(), key)

			Expect(err).To(HaveOccurred())
			Expect(job).To(BeNil())

			kube.ClientSet.CoreV1().ConfigMaps("default").Delete(context.Background(), name, metav1.DeleteOptions{})
		})

		It("should fail to create a run due to existing job", func() {
			j := batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "default",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								corev1.Container{
									Name:  "busybox",
									Image: "busybox",
								},
							},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			}

			kube.ClientSet.BatchV1().Jobs("default").Create(context.Background(), &j, metav1.CreateOptions{})

			job, err := run.CreateTerraformRun(context.Background(), key)

			Expect(err).To(HaveOccurred())
			Expect(job).To(BeNil())

			kube.ClientSet.BatchV1().Jobs("default").Delete(context.Background(), name, metav1.DeleteOptions{})
		})

		It("should return error if the job does not exist", func() {
			job, err := run.GetJobByRun(context.Background())

			Expect(err).To(HaveOccurred())
			Expect(job).To(BeNil())
		})

		It("should fail to delete a job that does not exist", func() {
			err := run.DeleteAfterCompletion(context.Background())

			Expect(err).To(HaveOccurred())
		})

		It("should fail to create a run due to ServiceAccount creation error", func() {
			kube.ClientSet.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("create", "serviceaccounts", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, &corev1.ServiceAccount{}, errors.New("Error creating service account")
			})

			job, err := run.CreateTerraformRun(context.Background(), key)

			Expect(err).To(HaveOccurred())
			Expect(job).To(BeNil())
		})

		It("should fail to create a run due to checking if ServiceAccount exist", func() {
			kube.ClientSet.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("get", "serviceaccounts", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, &corev1.ServiceAccount{}, errors.New("Error getting service account")
			})

			job, err := run.CreateTerraformRun(context.Background(), key)

			Expect(err).To(HaveOccurred())
			Expect(job).To(BeNil())
		})

		It("should fail to create a run due to RoleBinding creation error", func() {
			kube.ClientSet.RbacV1().(*fakerbacv1.FakeRbacV1).PrependReactor("create", "rolebindings", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, &rbacv1.RoleBinding{}, errors.New("Error creating role binding")
			})

			job, err := run.CreateTerraformRun(context.Background(), key)

			Expect(err).To(HaveOccurred())
			Expect(job).To(BeNil())
		})

		It("should fail to create a run due to checking if RoleBinding exist", func() {
			kube.ClientSet.RbacV1().(*fakerbacv1.FakeRbacV1).PrependReactor("get", "rolebindings", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, &rbacv1.RoleBinding{}, errors.New("Error getting role binding")
			})

			job, err := run.CreateTerraformRun(context.Background(), key)

			Expect(err).To(HaveOccurred())
			Expect(job).To(BeNil())
		})

		It("should fail to create a run due to Secret creation error", func() {
			kube.ClientSet.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("create", "secrets", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, &corev1.Secret{}, errors.New("Error creating secret")
			})

			job, err := run.CreateTerraformRun(context.Background(), key)

			Expect(err).To(HaveOccurred())
			Expect(job).To(BeNil())
		})

		It("should fail to create a run due to checking if Secret exist", func() {
			kube.ClientSet.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("get", "secrets", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, &corev1.Secret{}, errors.New("Error getting secret")
			})

			job, err := run.CreateTerraformRun(context.Background(), key)

			Expect(err).To(HaveOccurred())
			Expect(job).To(BeNil())
		})

		It("will fail to cleanup resources if job fail to delete", func() {
			kube.ClientSet.BatchV1().(*fakebatchv1.FakeBatchV1).PrependReactor("delete", "jobs", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, &batchv1.Job{}, errors.New("Error deleting job")
			})

			run.Status.RunID = "1234"
			run.Status.PreviousRunID = "jawd12"

			job, err := run.CreateTerraformRun(context.Background(), key)

			Expect(err).ToNot(HaveOccurred())
			Expect(job).ToNot(BeNil())

			err = run.CleanupResources(context.Background())
			Expect(err).To(HaveOccurred())
		})

		It("will fail to cleanup resources if configmap fail to delete", func() {
			kube.ClientSet.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("delete", "configmaps", func(action testing.Action) (handled bool, ret runtime.Object, err error) {
				return true, &corev1.ConfigMap{}, errors.New("Error deleting config map")
			})

			err := run.CleanupResources(context.Background())
			Expect(err).To(HaveOccurred())
		})
	})
})

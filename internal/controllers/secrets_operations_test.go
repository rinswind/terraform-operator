package controllers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kuptan/terraform-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kubernetes Secrets", func() {
	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	Context("Secrets", func() {
		key := types.NamespacedName{
			Name:      "bar",
			Namespace: "default",
		}

		t := &TerraformManipulator{&v1alpha1.Terraform{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bar",
				Namespace: "default",
			},
			Spec: v1alpha1.TerraformSpec{
				TerraformVersion: "1.0.2",
				Module: v1alpha1.Module{
					Source:  "IbraheemAlSaady/test/module",
					Version: "0.0.2",
				},
				Destroy:             false,
				DeleteCompletedJobs: false,
			},
			Status: v1alpha1.TerraformStatus{
				RunID: "1234",
			},
		}}

		expectedSecretName := key.Name + "-outputs"

		It("should create the secret successfully", func() {
			secret, err := t.createSecretForOutputs(context.Background(), key)

			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
			Expect(secret.Name).To(Equal(expectedSecretName))
		})

		// It("should retutrn the secret if exist", func() {
		// 	secret, err := getSecret(context.Background(), expectedSecretName, key.Namespace)

		// 	Expect(err).ToNot(HaveOccurred())
		// 	Expect(secret).ToNot(BeNil())
		// 	Expect(secret.Name).To(Equal(expectedSecretName))
		// })

		It("should not fail to create a secret that already exist", func() {
			secret, err := t.createSecretForOutputs(context.Background(), key)

			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
			Expect(secret.Name).To(Equal(expectedSecretName))
		})

		// It("should return nils if a secret was not found", func() {
		// 	secrets := kube.ClientSet.CoreV1().Secrets(key.Namespace)

		// 	deletePolicy := metav1.DeletePropagationForeground

		// 	secrets.Delete(context.Background(), expectedSecretName, metav1.DeleteOptions{
		// 		PropagationPolicy: &deletePolicy,
		// 	})

		// 	secret, err := getSecret(context.Background(), expectedSecretName, key.Namespace)

		// 	Expect(err).ToNot(HaveOccurred())
		// 	Expect(secret).To(BeNil())
		// })
	})
})

package controllers

import (
	"context"

	"github.com/kuptan/terraform-operator/api/v1alpha1"
	"github.com/kuptan/terraform-operator/internal/resources"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kubernetes ConfigMaps", func() {
	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	Context("ConfigMap", func() {
		key := types.NamespacedName{
			Name:      "bar",
			Namespace: "default",
		}

		t := &resources.TerraformManipulator{Terraform: &v1alpha1.Terraform{
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

		It("should create the configmap successfully", func() {
			cfg, err := t.createConfigMapForModule(context.Background(), key)

			expectedName := "bar-1234"

			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).ToNot(BeNil())
			Expect(cfg.Name).To(Equal(expectedName))
		})

		It("should delete the configmap successfully", func() {
			err := t.deleteConfigMapByRun(context.Background(), key.Name, key.Namespace, t.Status.RunID)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should return an error if the configmap does not exist", func() {
			err := t.deleteConfigMapByRun(context.Background(), key.Name, key.Namespace, t.Status.RunID)

			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})
})

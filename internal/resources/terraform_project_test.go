package resources

import (
	"github.com/kuptan/terraform-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Terraform Module", func() {
	expectedFile := `terraform {
	
		required_version = "~> 1.0.2"
	}
	variable "length" {}
	
	## additional-blocks
	
	module "operator" {
		source = "IbraheemAlSaady/test/module"
		version = "0.0.1"
		length = var.length
	}`
	Context("Terraform Template", func() {
		It("should generate the final module", func() {
			run := &v1alpha1.Terraform{
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
			}

			tpl, err := getTerraformModuleFromTemplate(run)

			tplString := string(tpl)

			Expect(err).ToNot(HaveOccurred())

			Expect(tplString).To(Equal(expectedFile))
		})
	})
})

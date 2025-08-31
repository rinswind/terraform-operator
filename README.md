# Terraform Operator

>[!NOTE]
> This project is based on [terraform-operator](https://github.com/kuptan/terraform-operator)
> Substantial modifications have been made that touch nearly every line of code

The Terraform Operator provides support to run Terraform modules in Kubernetes in a declarative way as a [Kubernetes manifest](https://kubernetes.io/docs/concepts/cluster-administration/manage-deployment/).

This project makes running a Terraform module, Kubernetes native through a single Kubernetes [CRD](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/). You can run the manifest with kubectl, Terraform, GitOps tools, etc...

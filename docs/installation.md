---
layout: default
title: Installation
nav_order: 2
---

# Installation
You can install the Terraform Operator either with `Helm` or directly apply the manifests with `kubectl`

**Helm**

```bash
  helm repo add rinswind https://rinswind.github.io/helm-charts
  helm install terraform-operator rinswind/terraform-operator
```

The Helm Chart source code can be found [here](https://github.com/rinswind/helm-charts/tree/master/charts/terraform-operator)

**Kubectl**

```bash
  kubectl apply -k https://github.com/rinswind/terraform-operator/config/crd 
  kubectl apply -k https://github.com/rinswind/terraform-operator/config/manifest
```

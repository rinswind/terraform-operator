package resources

import (
	"bytes"
	"fmt"
	"text/template"
)

// getTerraformModuleFromTemplate generates the Terraform module template
func (t *TerraformManipulator) GetTerraformModuleFromTemplate() ([]byte, error) {
	tfTemplate, err := template.New("main.tf").Parse(`terraform {
		{{- if .Spec.Backend }}
		{{.Spec.Backend}}
		{{- end}}
	
		required_version = "~> {{.Spec.TerraformVersion}}"
	}

	{{- if .Spec.ProvidersConfig }}
	{{.Spec.ProvidersConfig}}
	{{- end}}
	
	{{- range .Spec.Variables}}
	{{- if not .EnvironmentVariable }}
	variable "{{.Key}}" {}
	{{- end}}
	{{- end}}
	
	## additional-blocks
	
	module "operator" {
		source = "{{.Spec.Module.Source}}"
		
		{{- if .Spec.Module.Version }}
		version = "{{.Spec.Module.Version}}"
		{{- end}}
	
		{{- range .Spec.Variables}}
		{{- if not .EnvironmentVariable }}
		{{.Key}} = var.{{.Key}}
		{{- end}}
		{{- end}}
	}
	
	{{- range .Spec.Outputs}}
	output "{{.Key}}" {
		value = module.operator.{{.ModuleOutputName}}
	}
	{{- end}}`)

	if err != nil {
		return nil, err
	}
	var tpl bytes.Buffer

	if err := tfTemplate.Execute(&tpl, t); err != nil {
		return nil, err
	}

	return tpl.Bytes(), nil
}

// setBackendCfgIfNotExist sets the default backend to Kunernetes if not provided
func (t *TerraformManipulator) setBackendCfgIfNotExist() {
	if t.Spec.Backend == "" {
		t.Spec.Backend = fmt.Sprintf(`backend "kubernetes" {
  secret_suffix     = "%s"
  in_cluster_config = true
  namespace         = "%s"
}
`, t.ObjectMeta.Name, t.ObjectMeta.Namespace)
	}
}

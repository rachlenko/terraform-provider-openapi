package openapi

import (
	"github.com/dikhan/terraform-provider-openapi/utils/terraform_docs_generator/openapi/templates/zendesk"
	"io"
)

// TerraformProviderDocumentation defines the attributes needed to generate Terraform provider documentation
type TerraformProviderDocumentation struct {
	ProviderName                string
	ProviderInstallation        ProviderInstallation
	ProviderConfiguration       ProviderConfiguration
	ProviderResources           ProviderResources
	DataSources                 DataSources
	ShowSpecialTermsDefinitions bool
}

func (t TerraformProviderDocumentation) RenderHTML(w io.Writer) error {
	return t.renderZendeskHTML(w, zendesk.TableOfContentsTmpl, zendesk.ProviderInstallationTmpl, zendesk.ProviderConfigurationTmpl, zendesk.ProviderResourcesTmpl, zendesk.DataSourcesTmpl, zendesk.SpecialTermsTmpl)
}

// RenderZendeskHTML renders the documentation in HTML
func (t TerraformProviderDocumentation) renderZendeskHTML(w io.Writer, tableOfContentsTemplate, providerInstallationTemplate, providerConfigurationTemplate, providerResourcesConfiguration, providerDatSourcesTemplate, specialTermsDefinitionsTemplate string) error {
	err := Render(w, "TerraformProviderDocTableOfContents", tableOfContentsTemplate, t)
	if err != nil {
		return err
	}
	err = t.ProviderInstallation.Render(w, providerInstallationTemplate)
	if err != nil {
		return err
	}
	err = t.ProviderConfiguration.Render(w, providerConfigurationTemplate)
	if err != nil {
		return err
	}
	err = t.ProviderResources.Render(w, providerResourcesConfiguration)
	if err != nil {
		return err
	}
	err = t.DataSources.Render(w, providerDatSourcesTemplate)
	if err != nil {
		return err
	}
	err = Render(w, "TerraformProviderDocSpecialTermsDefinitions", specialTermsDefinitionsTemplate, t)
	if err != nil {
		return err
	}
	return nil
}

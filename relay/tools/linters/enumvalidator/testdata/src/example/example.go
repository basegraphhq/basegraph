package example

type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderGitLab Provider = "gitlab"
)

type Capability string

const (
	CapabilityCodeRepo Capability = "code_repo"
)

type CredentialType string

const (
	CredentialTypeUserOAuth CredentialType = "user_oauth"
)

type Integration struct {
	Provider Provider
}

type IntegrationCredential struct {
	CredentialType CredentialType
}

func bad() {
	i := &Integration{}
	i.Provider = "bitbucket" // want "enum field Provider assigned string literal"

	c := &IntegrationCredential{}
	c.CredentialType = "api_key" // want "enum field CredentialType assigned string literal"
}

func good() {
	i := &Integration{}
	i.Provider = ProviderGitHub // OK: using constant

	c := &IntegrationCredential{}
	c.CredentialType = CredentialTypeUserOAuth // OK: using constant
}

func alsoGood() {
	// OK: Variable, not literal
	provider := ProviderGitHub
	i := &Integration{Provider: provider}
	_ = i
}

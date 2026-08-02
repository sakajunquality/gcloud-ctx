package adc

import (
	"fmt"
	"regexp"
)

// impersonationURLRE parses the URL format gcloud writes into
// service_account_impersonation_url.
var impersonationURLRE = regexp.MustCompile(`^https://iamcredentials\.([^/]+)/v1/projects/-/serviceAccounts/([^:]+):generateAccessToken$`)

// ImpersonationURL builds the service_account_impersonation_url gcloud
// writes for serviceAccount under universeDomain.
func ImpersonationURL(universeDomain, serviceAccount string) string {
	return fmt.Sprintf("https://iamcredentials.%s/v1/projects/-/serviceAccounts/%s:generateAccessToken", universeDomain, serviceAccount)
}

// ParseImpersonationURL extracts the universe domain and target service
// account email from a service_account_impersonation_url as written by
// gcloud.
func ParseImpersonationURL(url string) (universeDomain, serviceAccount string, err error) {
	m := impersonationURLRE.FindStringSubmatch(url)
	if m == nil {
		return "", "", fmt.Errorf("unrecognized impersonation URL %q", url)
	}
	return m[1], m[2], nil
}

// TargetServiceAccount is a convenience for ParseImpersonationURL callers
// that only need the service account.
func (c *Credential) TargetServiceAccount() (string, error) {
	_, sa, err := ParseImpersonationURL(c.ImpersonationURL())
	return sa, err
}

// acceptedBaseTypes are the credential types gcloud accepts as
// source_credentials for a freshly synthesized impersonation chain.
var acceptedBaseTypes = map[Type]bool{
	TypeAuthorizedUser:                true,
	TypeServiceAccount:                true,
	TypeExternalAccountAuthorizedUser: true,
}

// ResolveBase unwraps a single level of impersonated_service_account (gcloud
// never nests impersonation chains) and validates the result is an accepted
// base credential type for building a new impersonation chain on top of.
func ResolveBase(c *Credential) (*Credential, error) {
	base := c
	if c.Type() == TypeImpersonatedServiceAccount {
		var err error
		base, err = c.SourceCredentials()
		if err != nil {
			return nil, err
		}
	}
	if !acceptedBaseTypes[base.Type()] {
		return nil, fmt.Errorf("unsupported base credential type %q; run 'gcloud auth application-default login' first", base.Type())
	}
	return base, nil
}

// ImpersonateOptions parameterizes Synthesize.
type ImpersonateOptions struct {
	// ServiceAccount is the target service account email.
	ServiceAccount string
	// Delegates is the delegation chain toward ServiceAccount, in order.
	// May be nil/empty.
	Delegates []string
	// QuotaProject, if non-empty, is recorded as quota_project_id.
	QuotaProject string
	// Source is the base credential (already resolved via ResolveBase, i.e.
	// not itself an impersonated_service_account) to embed as
	// source_credentials.
	Source *Credential
}

// Synthesize builds the impersonated_service_account ADC document gcloud
// itself would write for the given options.
func Synthesize(opts ImpersonateOptions) (*Credential, error) {
	if opts.Source == nil {
		return nil, fmt.Errorf("synthesize impersonated credential: missing base credentials")
	}
	if opts.ServiceAccount == "" {
		return nil, fmt.Errorf("synthesize impersonated credential: missing target service account")
	}

	delegates := make([]any, len(opts.Delegates))
	for i, d := range opts.Delegates {
		delegates[i] = d
	}

	raw := map[string]any{
		"delegates":                         delegates,
		"service_account_impersonation_url": ImpersonationURL(opts.Source.UniverseDomain(), opts.ServiceAccount),
		"source_credentials":                opts.Source.raw,
		"type":                              string(TypeImpersonatedServiceAccount),
	}
	if opts.QuotaProject != "" {
		raw["quota_project_id"] = opts.QuotaProject
	}
	return &Credential{raw: raw}, nil
}

// Unimpersonate returns the credential to store after clearing impersonation
// (impersonate --clear): the unwrapped source_credentials if c is
// impersonated_service_account, or c unchanged otherwise.
func Unimpersonate(c *Credential) (*Credential, error) {
	if c.Type() != TypeImpersonatedServiceAccount {
		return c, nil
	}
	return c.SourceCredentials()
}

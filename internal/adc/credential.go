// Package adc models Application Default Credentials JSON files: reading and
// type-sniffing them, unwrapping impersonation chains, synthesizing new
// impersonated_service_account credentials exactly as gcloud does, and
// installing them atomically as the live ADC file.
package adc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Type identifies the "type" field of an ADC JSON document.
type Type string

// The credential types gcloud itself writes to an ADC JSON file.
const (
	TypeAuthorizedUser                Type = "authorized_user"
	TypeServiceAccount                Type = "service_account"
	TypeImpersonatedServiceAccount    Type = "impersonated_service_account"
	TypeExternalAccount               Type = "external_account"
	TypeExternalAccountAuthorizedUser Type = "external_account_authorized_user"
)

// defaultUniverseDomain is used when a credential omits universe_domain,
// matching google-auth's own default.
const defaultUniverseDomain = "googleapis.com"

// Credential is a parsed ADC JSON document. It's backed by the full decoded
// object so that fields gcloud-ctx doesn't specifically model still survive
// a read-modify-write round trip.
type Credential struct {
	raw map[string]any
}

// Parse decodes an ADC JSON document.
func Parse(data []byte) (*Credential, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ADC JSON: %w", err)
	}
	return &Credential{raw: raw}, nil
}

// Read parses the ADC JSON document at path.
func Read(path string) (*Credential, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ADC file: %w", err)
	}
	c, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Type returns the credential's "type" field.
func (c *Credential) Type() Type {
	t, _ := c.raw["type"].(string)
	return Type(t)
}

// UniverseDomain returns the credential's "universe_domain" field, defaulting
// to "googleapis.com" when absent.
func (c *Credential) UniverseDomain() string {
	if v, ok := c.raw["universe_domain"].(string); ok && v != "" {
		return v
	}
	return defaultUniverseDomain
}

// String returns a field of the credential as a string, or "" if absent or
// not a string. It's a convenience for display code (e.g. "show").
func (c *Credential) String(field string) string {
	v, _ := c.raw[field].(string)
	return v
}

// SourceCredentials returns the embedded source_credentials as a Credential.
// It's an error to call this on anything but an
// impersonated_service_account credential.
func (c *Credential) SourceCredentials() (*Credential, error) {
	if c.Type() != TypeImpersonatedServiceAccount {
		return nil, fmt.Errorf("credential type %q has no source_credentials", c.Type())
	}
	src, ok := c.raw["source_credentials"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("malformed source_credentials in impersonated_service_account credential")
	}
	return &Credential{raw: src}, nil
}

// WithSourceCredentials returns a copy of an impersonated_service_account
// credential whose source_credentials is replaced by base, leaving the
// impersonation target, delegates, and quota project untouched. Used when a
// base user credential has been re-obtained (gcloud-ctx refresh) and
// dependent impersonation snapshots must be rebuilt on top of it.
func (c *Credential) WithSourceCredentials(base *Credential) (*Credential, error) {
	if c.Type() != TypeImpersonatedServiceAccount {
		return nil, fmt.Errorf("credential type %q has no source_credentials", c.Type())
	}
	if base == nil {
		return nil, fmt.Errorf("missing base credentials")
	}
	// Deep-copy via a marshal/parse round trip so neither c nor base shares
	// mutable state with the result.
	data, err := c.Marshal()
	if err != nil {
		return nil, err
	}
	out, err := Parse(data)
	if err != nil {
		return nil, err
	}
	baseData, err := base.Marshal()
	if err != nil {
		return nil, err
	}
	baseCopy, err := Parse(baseData)
	if err != nil {
		return nil, err
	}
	out.raw["source_credentials"] = baseCopy.raw
	return out, nil
}

// ImpersonationURL returns the "service_account_impersonation_url" field. It
// is only meaningful for impersonated_service_account credentials.
func (c *Credential) ImpersonationURL() string {
	return c.String("service_account_impersonation_url")
}

// Marshal serializes the credential with sorted keys and 2-space indent,
// matching gcloud's own json.dumps(sort_keys=True, indent=2) byte for byte.
// HTML-unsafe characters ('<', '>', '&') are emitted literally rather than
// escaped, since Go's default JSON encoding escapes them for safe embedding
// in HTML — behavior Python's json.dumps doesn't have and gcloud's own
// output never exhibits.
func (c *Credential) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c.raw); err != nil {
		return nil, fmt.Errorf("marshal ADC JSON: %w", err)
	}
	// json.Encoder.Encode always appends a trailing newline; MarshalIndent
	// (and Python's json.dumps) doesn't, so trim it for a byte-for-byte
	// match with gcloud's own output.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

package adc

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_typeSniffing(t *testing.T) {
	tests := []struct {
		file string
		want Type
	}{
		{"authorized_user.json", TypeAuthorizedUser},
		{"service_account.json", TypeServiceAccount},
		{"impersonated_service_account.json", TypeImpersonatedServiceAccount},
		{"external_account.json", TypeExternalAccount},
		{"external_account_authorized_user.json", TypeExternalAccountAuthorizedUser},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			c, err := Read(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if got := c.Type(); got != tt.want {
				t.Errorf("Type() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParse_invalidJSON(t *testing.T) {
	if _, err := Parse([]byte("not json")); err == nil {
		t.Error("Parse(invalid) error = nil, want error")
	}
}

func TestCredential_UniverseDomain(t *testing.T) {
	t.Run("defaults when absent", func(t *testing.T) {
		c, err := Read(filepath.Join("testdata", "authorized_user.json"))
		if err != nil {
			t.Fatal(err)
		}
		if got := c.UniverseDomain(); got != "googleapis.com" {
			t.Errorf("UniverseDomain() = %q, want googleapis.com", got)
		}
	})

	t.Run("uses explicit field", func(t *testing.T) {
		c, err := Parse([]byte(`{"type":"authorized_user","universe_domain":"my-universe.example.com"}`))
		if err != nil {
			t.Fatal(err)
		}
		if got := c.UniverseDomain(); got != "my-universe.example.com" {
			t.Errorf("UniverseDomain() = %q, want my-universe.example.com", got)
		}
	})
}

func TestCredential_SourceCredentials(t *testing.T) {
	c, err := Read(filepath.Join("testdata", "impersonated_service_account.json"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := c.SourceCredentials()
	if err != nil {
		t.Fatalf("SourceCredentials() error = %v", err)
	}
	if src.Type() != TypeAuthorizedUser {
		t.Errorf("SourceCredentials().Type() = %q, want authorized_user", src.Type())
	}

	notImpersonated, err := Read(filepath.Join("testdata", "authorized_user.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notImpersonated.SourceCredentials(); err == nil {
		t.Error("SourceCredentials() on non-impersonated credential error = nil, want error")
	}
}

func TestCredential_Marshal_disablesHTMLEscaping(t *testing.T) {
	c, err := Parse([]byte(`{"type":"authorized_user","quota_project_id":"a&b<c>d"}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	// With HTML escaping disabled, the raw characters appear verbatim...
	if !strings.Contains(string(data), "a&b<c>d") {
		t.Errorf("Marshal() = %s, want the literal HTML-unsafe characters preserved, not escaped", data)
	}
	// ...instead of Go's default &/</> escape sequences.
	for _, escapeSeq := range []string{"\\u0026", "\\u003c", "\\u003e"} {
		if strings.Contains(string(data), escapeSeq) {
			t.Errorf("Marshal() = %s, want HTML escaping disabled (found %s)", data, escapeSeq)
		}
	}
}

func TestCredential_Marshal_noTrailingNewline(t *testing.T) {
	c, err := Read(filepath.Join("testdata", "authorized_user.json"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(data) == 0 || data[len(data)-1] == '\n' {
		t.Errorf("Marshal() ends with a trailing newline, want none (byte-for-byte match with gcloud's json.dumps)")
	}
}

func TestCredential_Marshal_roundTrip(t *testing.T) {
	c, err := Read(filepath.Join("testdata", "impersonated_service_account.json"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	reparsed, err := Parse(data)
	if err != nil {
		t.Fatalf("re-Parse of Marshal() output error = %v", err)
	}
	if reparsed.Type() != TypeImpersonatedServiceAccount {
		t.Errorf("round-tripped Type() = %q, want impersonated_service_account", reparsed.Type())
	}
}

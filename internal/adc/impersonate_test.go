package adc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImpersonationURL(t *testing.T) {
	got := ImpersonationURL("googleapis.com", "sa@my-project.iam.gserviceaccount.com")
	want := "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@my-project.iam.gserviceaccount.com:generateAccessToken"
	if got != want {
		t.Errorf("ImpersonationURL() = %q, want %q", got, want)
	}
}

func TestParseImpersonationURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantUniv string
		wantSA   string
		wantErr  bool
	}{
		{
			name:     "googleapis.com",
			url:      "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@my-project.iam.gserviceaccount.com:generateAccessToken",
			wantUniv: "googleapis.com",
			wantSA:   "sa@my-project.iam.gserviceaccount.com",
		},
		{
			name:     "custom universe domain",
			url:      "https://iamcredentials.my-universe.example.com/v1/projects/-/serviceAccounts/sa@my-project.iam.gserviceaccount.com:generateAccessToken",
			wantUniv: "my-universe.example.com",
			wantSA:   "sa@my-project.iam.gserviceaccount.com",
		},
		{
			name:    "malformed",
			url:     "https://example.com/not-an-impersonation-url",
			wantErr: true,
		},
		{
			name:    "empty",
			url:     "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			univ, sa, err := ParseImpersonationURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseImpersonationURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if univ != tt.wantUniv || sa != tt.wantSA {
				t.Errorf("ParseImpersonationURL(%q) = (%q, %q), want (%q, %q)", tt.url, univ, sa, tt.wantUniv, tt.wantSA)
			}
		})
	}
}

func TestCredential_TargetServiceAccount(t *testing.T) {
	c, err := Read(filepath.Join("testdata", "impersonated_service_account.json"))
	if err != nil {
		t.Fatal(err)
	}
	sa, err := c.TargetServiceAccount()
	if err != nil {
		t.Fatalf("TargetServiceAccount() error = %v", err)
	}
	if want := "target@my-project.iam.gserviceaccount.com"; sa != want {
		t.Errorf("TargetServiceAccount() = %q, want %q", sa, want)
	}
}

func TestResolveBase(t *testing.T) {
	tests := []struct {
		file    string
		want    Type
		wantErr bool
	}{
		{"authorized_user.json", TypeAuthorizedUser, false},
		{"service_account.json", TypeServiceAccount, false},
		{"external_account_authorized_user.json", TypeExternalAccountAuthorizedUser, false},
		{"impersonated_service_account.json", TypeAuthorizedUser, false}, // unwraps one level
		{"external_account.json", "", true},                              // not an accepted base type
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			c, err := Read(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			base, err := ResolveBase(c)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveBase(%s) error = %v, wantErr %v", tt.file, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if base.Type() != tt.want {
				t.Errorf("ResolveBase(%s).Type() = %q, want %q", tt.file, base.Type(), tt.want)
			}
		})
	}
}

func TestSynthesize(t *testing.T) {
	source, err := Read(filepath.Join("testdata", "authorized_user.json"))
	if err != nil {
		t.Fatal(err)
	}

	c, err := Synthesize(ImpersonateOptions{
		ServiceAccount: "target@my-project.iam.gserviceaccount.com",
		Delegates:      []string{"delegate1@my-project.iam.gserviceaccount.com"},
		QuotaProject:   "my-quota-project",
		Source:         source,
	})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	got, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "synthesized_impersonated.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("Synthesize() output does not match golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSynthesize_noDelegatesNoQuotaProject(t *testing.T) {
	source, err := Read(filepath.Join("testdata", "authorized_user.json"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Synthesize(ImpersonateOptions{
		ServiceAccount: "target@my-project.iam.gserviceaccount.com",
		Source:         source,
	})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	got, err := c.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "synthesized_impersonated_minimal.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("Synthesize() output does not match golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSynthesize_missingSource(t *testing.T) {
	if _, err := Synthesize(ImpersonateOptions{ServiceAccount: "sa@my-project.iam.gserviceaccount.com"}); err == nil {
		t.Error("Synthesize() without Source error = nil, want error")
	}
}

func TestUnimpersonate(t *testing.T) {
	t.Run("unwraps impersonated credential", func(t *testing.T) {
		c, err := Read(filepath.Join("testdata", "impersonated_service_account.json"))
		if err != nil {
			t.Fatal(err)
		}
		base, err := Unimpersonate(c)
		if err != nil {
			t.Fatalf("Unimpersonate() error = %v", err)
		}
		if base.Type() != TypeAuthorizedUser {
			t.Errorf("Unimpersonate().Type() = %q, want authorized_user", base.Type())
		}
	})

	t.Run("passes through non-impersonated credential", func(t *testing.T) {
		c, err := Read(filepath.Join("testdata", "service_account.json"))
		if err != nil {
			t.Fatal(err)
		}
		got, err := Unimpersonate(c)
		if err != nil {
			t.Fatalf("Unimpersonate() error = %v", err)
		}
		if got.Type() != TypeServiceAccount {
			t.Errorf("Unimpersonate().Type() = %q, want service_account", got.Type())
		}
	})
}

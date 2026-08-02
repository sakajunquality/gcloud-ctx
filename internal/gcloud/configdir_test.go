package gcloud

import (
	"path/filepath"
	"runtime"
	"testing"
)

// clearConfigEnv unsets every env var ConfigDir consults, returning them via
// t.Setenv defaults so each subtest starts from a clean slate.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{ConfigEnvVar, "APPDATA", "SystemDrive", "XDG_CONFIG_HOME"} {
		t.Setenv(k, "")
	}
}

func TestConfigDir(t *testing.T) {
	t.Run("CLOUDSDK_CONFIG takes priority", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv(ConfigEnvVar, "/custom/config/dir")
		got, err := ConfigDir()
		if err != nil {
			t.Fatalf("ConfigDir() error = %v", err)
		}
		if got != "/custom/config/dir" {
			t.Errorf("ConfigDir() = %q, want %q", got, "/custom/config/dir")
		}
	})

	t.Run("XDG_CONFIG_HOME is ignored", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("XDG_CONFIG_HOME", "/xdg/config")
		t.Setenv("HOME", "/home/user")
		got, err := ConfigDir()
		if err != nil {
			t.Fatalf("ConfigDir() error = %v", err)
		}
		if runtime.GOOS != "windows" {
			want := filepath.Join("/home/user", ".config", "gcloud")
			if got != want {
				t.Errorf("ConfigDir() = %q, want %q (XDG_CONFIG_HOME must be ignored)", got, want)
			}
		}
	})

	if runtime.GOOS == "windows" {
		t.Run("windows APPDATA", func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("APPDATA", `C:\Users\test\AppData\Roaming`)
			got, err := ConfigDir()
			if err != nil {
				t.Fatalf("ConfigDir() error = %v", err)
			}
			want := filepath.Join(`C:\Users\test\AppData\Roaming`, "gcloud")
			if got != want {
				t.Errorf("ConfigDir() = %q, want %q", got, want)
			}
		})

		t.Run("windows falls back to SystemDrive", func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("SystemDrive", `D:`)
			got, err := ConfigDir()
			if err != nil {
				t.Fatalf("ConfigDir() error = %v", err)
			}
			// Deliberately not filepath.Join(`D:`, "gcloud") here: that's the
			// exact unrooted-path bug ("D:gcloud") this fallback must avoid.
			want := `D:\gcloud`
			if got != want {
				t.Errorf("ConfigDir() = %q, want %q (must be rooted)", got, want)
			}
		})

		t.Run("windows falls back to C: when nothing set", func(t *testing.T) {
			clearConfigEnv(t)
			got, err := ConfigDir()
			if err != nil {
				t.Fatalf("ConfigDir() error = %v", err)
			}
			want := `C:\gcloud`
			if got != want {
				t.Errorf("ConfigDir() = %q, want %q (must be rooted)", got, want)
			}
		})
	}
}

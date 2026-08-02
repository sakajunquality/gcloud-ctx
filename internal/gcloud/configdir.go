// Package gcloud resolves the gcloud config directory and manages named
// configurations (the "configurations/config_<name>" INI files under it),
// matching the behavior of the Cloud SDK's own config.py exactly.
package gcloud

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ConfigEnvVar is the environment variable gcloud itself honors to override
// the config directory.
const ConfigEnvVar = "CLOUDSDK_CONFIG"

// ConfigDir resolves the gcloud config directory the same way Cloud SDK's
// config.py:_GetGlobalConfigDir does:
//
//	$CLOUDSDK_CONFIG if set
//	else on Windows: %APPDATA%\gcloud, falling back to %SystemDrive%\gcloud
//	else: $HOME/.config/gcloud
//
// XDG_CONFIG_HOME is deliberately not honored: real gcloud ignores it too.
func ConfigDir() (string, error) {
	if dir := os.Getenv(ConfigEnvVar); dir != "" {
		return dir, nil
	}

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "gcloud"), nil
		}
		sysDrive := os.Getenv("SystemDrive")
		if sysDrive == "" {
			sysDrive = "C:"
		}
		// Deliberately not filepath.Join(sysDrive, "gcloud"): when the first
		// element is exactly a bare drive letter ("C:"), Join's Windows
		// implementation treats it as "relative to the current directory on
		// that drive" and concatenates without a separator, producing the
		// unrooted "C:gcloud" instead of "C:\gcloud". Join a
		// separator-terminated drive instead so the result is always rooted.
		return filepath.Join(sysDrive+`\`, "gcloud"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "gcloud"), nil
}

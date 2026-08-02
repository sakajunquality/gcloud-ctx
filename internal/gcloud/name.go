package gcloud

import (
	"fmt"
	"regexp"
)

// NoneConfig is gcloud's virtual, file-less, read-only configuration. It is
// a legal switch target ("gcloud-ctx --unset") but never a legal name for
// create/rename.
const NoneConfig = "NONE"

// nameRE matches the configuration name grammar gcloud enforces.
var nameRE = regexp.MustCompile(`^[a-z][-a-z0-9]*$`)

// IsNone reports whether name is the reserved NONE pseudo-configuration.
func IsNone(name string) bool {
	return name == NoneConfig
}

// ValidateName reports whether name is a syntactically valid configuration
// name. It rejects NoneConfig; callers that accept NONE as a special switch
// target must check IsNone themselves before calling ValidateName.
func ValidateName(name string) error {
	if IsNone(name) {
		return fmt.Errorf("%q is reserved and cannot be used as a configuration name", name)
	}
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid configuration name %q: must match %s", name, nameRE.String())
	}
	return nil
}

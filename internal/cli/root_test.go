package cli

import (
	"strings"
	"testing"
)

// TestRootHelp_hasKubectxSynopsis covers item 11: the root command's Long
// help text must carry a short kubectx-grammar synopsis with concrete
// examples covering list/switch/-/rename/delete.
func TestRootHelp_hasKubectxSynopsis(t *testing.T) {
	testEnv(t)

	stdout, _, err := run(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Examples",
		"gcloud-ctx staging", // switch
		"gcloud-ctx -",       // previous
		"new=old",            // rename
		"-d staging",         // delete
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("--help output = %q, want it to contain %q", stdout, want)
		}
	}
}

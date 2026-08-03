package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
)

// NewRootCmd builds the gcloud-ctx root command implementing the kubectx-
// style grammar described in DESIGN.md, plus its subcommands.
func NewRootCmd(version string) *cobra.Command {
	var (
		currentFlag bool
		longFlag    bool
		deleteFlag  bool
		unsetFlag   bool
	)

	root := &cobra.Command{
		Use:   "gcloud-ctx [NAME | - | NEW=OLD]",
		Short: "Switch between gcloud named configurations (and their ADC) fast",
		Long: strings.TrimSpace(`
gcloud-ctx switches between gcloud named configurations and, unlike plain
"gcloud config configurations activate", keeps Application Default
Credentials and service account impersonation in sync with the switch.

Examples (kubectx-style grammar):
  gcloud-ctx                  List contexts (interactive picker on a TTY)
  gcloud-ctx staging          Switch to context "staging"
  gcloud-ctx -                Switch to the previous context
  gcloud-ctx -l               List contexts as a table
  gcloud-ctx new=old          Rename context "old" to "new"
  gcloud-ctx -d staging       Delete context "staging"
`),
		Version:           version,
		SilenceUsage:      true,
		SilenceErrors:     true,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeContextNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := newApp(cmd)
			if err != nil {
				return err
			}
			return runRoot(a, args, currentFlag, longFlag, deleteFlag, unsetFlag)
		},
	}

	root.Flags().BoolVarP(&currentFlag, "current", "c", false, "print the current context name")
	root.Flags().BoolVarP(&longFlag, "long", "l", false, "list contexts as a table")
	root.Flags().BoolVarP(&deleteFlag, "delete", "d", false, "delete the given context(s)")
	root.Flags().BoolVarP(&unsetFlag, "unset", "u", false, "activate gcloud's virtual NONE configuration")
	root.MarkFlagsMutuallyExclusive("current", "long", "delete", "unset")

	root.AddCommand(
		newCreateCmd(),
		newShowCmd(),
		newImpersonateCmd(),
		newADCCmd(),
		newEnvCmd(),
		newRefreshCmd(),
	)

	return root
}

func runRoot(a *app, args []string, current, long, del, unset bool) error {
	switch {
	case del:
		if len(args) == 0 {
			return fmt.Errorf("--delete requires at least one context name")
		}
		return a.deleteContexts(args)

	case unset:
		if len(args) != 0 {
			return fmt.Errorf("--unset takes no arguments")
		}
		return a.switchTo(gcloud.NoneConfig)

	case current:
		if len(args) != 0 {
			return fmt.Errorf("--current takes no arguments")
		}
		return a.printCurrent()

	case long:
		if len(args) != 0 {
			return fmt.Errorf("--long takes no arguments")
		}
		return a.listLong()

	case len(args) == 0:
		return a.listOrPick()

	case len(args) == 1 && args[0] == "-":
		return a.switchToPrevious()

	case len(args) == 1:
		if newName, oldName, ok := parseRename(args[0]); ok {
			return a.rename(newName, oldName)
		}
		return a.switchTo(args[0])

	default:
		return fmt.Errorf("accepts at most 1 arg (or multiple with --delete), received %d", len(args))
	}
}

// parseRename recognizes the "<NEW>=<OLD>" rename syntax. Configuration
// names never contain "=", so any argument containing it is unambiguously
// rename syntax rather than a context name.
func parseRename(arg string) (newName, oldName string, ok bool) {
	idx := strings.Index(arg, "=")
	if idx < 0 {
		return "", "", false
	}
	return arg[:idx], arg[idx+1:], true
}

// completeContextNames is the root's ValidArgsFunction: it completes context
// names via the same gcloud.Store.List code path "gcloud-ctx" itself lists
// from.
func completeContextNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	gs, err := gcloud.Open()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	configs, err := gs.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := make([]string, 0, len(configs))
	for _, c := range configs {
		names = append(names, c.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

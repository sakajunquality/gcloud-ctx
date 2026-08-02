package cli

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/sakajunquality/gcloud-ctx/internal/gcloud"
	"github.com/sakajunquality/gcloud-ctx/internal/ui"
)

// parseErrorMarker is rendered in each property column of "-l" for a
// configuration whose backing INI file failed to parse (gcloud.Config.
// ParseErr), so a single corrupt file doesn't abort the whole listing —
// the name (and ADC status, which doesn't depend on the INI file) still
// render normally.
const parseErrorMarker = "<parse error>"

// listOrPick implements bare "gcloud-ctx": the interactive picker when both
// streams are terminals and GCLOUD_CTX_IGNORE_FZF is unset, else a plain
// sorted list of names, one per line.
func (a *app) listOrPick() error {
	configs, err := a.gs.List()
	if err != nil {
		return err
	}
	if len(configs) == 0 {
		fmt.Fprintln(a.errw, "No contexts found.")
		return nil
	}

	if ui.PickerEnabled(a.in, a.out) {
		return a.pick(configs)
	}
	return a.printPlainList(configs)
}

func (a *app) pick(configs []gcloud.Config) error {
	active, err := a.gs.EffectiveActiveName()
	if err != nil {
		return err
	}

	items := make([]ui.PickerItem, len(configs))
	for i, c := range configs {
		items[i] = ui.PickerItem{Name: c.Name, Preview: previewFor(c, c.Name == active)}
	}

	selected, err := ui.Pick(items)
	if err != nil {
		return err
	}
	return a.switchTo(selected)
}

func previewFor(c gcloud.Config, active bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Name:          %s\n", c.Name)
	fmt.Fprintf(&b, "Active:        %v\n", active)
	if c.ParseErr != nil {
		fmt.Fprintf(&b, "%s\n", parseErrorMarker)
		return b.String()
	}
	target, delegates := splitImpersonationChain(c.ImpersonateServiceAccount)
	fmt.Fprintf(&b, "Account:       %s\n", dash(sanitize(c.Account)))
	fmt.Fprintf(&b, "Project:       %s\n", dash(sanitize(c.Project)))
	fmt.Fprintf(&b, "Region:        %s\n", dash(sanitize(c.Region)))
	fmt.Fprintf(&b, "Zone:          %s\n", dash(sanitize(c.Zone)))
	fmt.Fprintf(&b, "Impersonation: %s\n", dash(sanitize(target)))
	fmt.Fprintf(&b, "Delegates:     %s\n", dash(sanitize(strings.Join(delegates, ", "))))
	return b.String()
}

func (a *app) printPlainList(configs []gcloud.Config) error {
	active, err := a.gs.EffectiveActiveName()
	if err != nil {
		return err
	}
	for _, c := range configs {
		name := c.Name
		if c.Name == active {
			name = a.style.Active(name)
		}
		fmt.Fprintln(a.out, name)
	}
	return nil
}

// listLong implements "gcloud-ctx -l/--long": a table of NAME, ACTIVE,
// ACCOUNT, PROJECT, IMPERSONATION, ADC. It never uses the interactive
// picker — the table itself is the explicit request.
//
// Column widths are computed by tabwriter from plain (uncolored) text only:
// ANSI escape codes are never fed into it, since tabwriter counts them as
// visible characters and would misalign every column whenever color is on.
// Instead, the whole table is rendered plain to an internal buffer first,
// and only the active row's NAME cell — already padded to the right width —
// has color spliced in afterward, once alignment no longer depends on it.
func (a *app) listLong() error {
	configs, err := a.gs.List()
	if err != nil {
		return err
	}
	active, err := a.gs.EffectiveActiveName()
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tACTIVE\tACCOUNT\tPROJECT\tIMPERSONATION\tADC")
	activeIdx := -1
	for i, c := range configs {
		activeMark := ""
		if c.Name == active {
			activeMark = "*"
			activeIdx = i
		}
		hasADC, err := a.ss.HasADC(c.Name)
		if err != nil {
			return err
		}
		if c.ParseErr != nil {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", c.Name, activeMark, parseErrorMarker, parseErrorMarker, parseErrorMarker, yesNo(hasADC))
			continue
		}
		target, _ := splitImpersonationChain(c.ImpersonateServiceAccount)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			c.Name, activeMark, dash(sanitize(c.Account)), dash(sanitize(c.Project)), dash(sanitize(target)), yesNo(hasADC))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if activeIdx >= 0 && activeIdx+1 < len(lines) {
		lines[activeIdx+1] = colorizeNamePrefix(lines[activeIdx+1], configs[activeIdx].Name, a.style)
	}
	for _, line := range lines {
		fmt.Fprintln(a.out, line)
	}
	return nil
}

// colorizeNamePrefix wraps the leading NAME field of line — already padded
// by tabwriter to the column's plain-text width — in style's active
// decoration, leaving the padding and remaining columns untouched.
func colorizeNamePrefix(line, name string, style ui.Style) string {
	if !strings.HasPrefix(line, name) {
		return line // defensive; the row always starts with its own name
	}
	return style.Active(name) + line[len(name):]
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

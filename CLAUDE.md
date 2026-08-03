# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`gcloud-ctx` is a kubectx-style context switcher for the `gcloud` CLI, written in Go (cobra). A "context" = a gcloud named configuration **plus** an optional per-context ADC (Application Default Credentials) snapshot. The differentiator over `gcloud config configurations activate`: switching also switches ADC and keeps service account impersonation consistent across both the gcloud property (`auth/impersonate_service_account`) and the ADC JSON (`impersonated_service_account` wrapper).

**`docs/DESIGN.md` is authoritative.** It specifies exact command semantics, error-message wording, file-write ordering (ADC-then-property, stage-then-commit around `active_config`), and gcloud interop facts verified against Cloud SDK source. Read the relevant section before changing behavior, and update it (plus README.md) when changing command surface or file/env-var semantics.

## Commands

```sh
go build ./...
go test -race ./...                              # full suite (what CI runs)
go test -race ./internal/cli -run TestSwitch     # single test
golangci-lint run                                # config in .golangci.yml (v2 format)
gofmt -l . && goimports -l .                     # formatting, also enforced by lint
```

## Architecture

```
main.go            # thin: version vars (ldflags), cli.Execute()
internal/cli/      # cobra commands; root implements the kubectx grammar
                   # (gcloud-ctx <NAME> switches, `-` previous, NEW=OLD rename, -d delete, ...)
internal/gcloud/   # config dir resolution, named-config store, INI property access
internal/adc/      # ADC file model: type sniffing, synthesize/unwrap impersonation,
                   # atomic 0600 install
internal/store/    # gcloud-ctx state: `previous` file, per-context ADC snapshots
internal/ui/       # color (NO_COLOR/FORCE_COLOR/TTY), fuzzy picker (go-fuzzyfinder)
```

Commands run through an `app` struct (`internal/cli/app.go`) bundling the gcloud store, state store, I/O streams, and color style — always write through `a.out`/`a.errw`, never `os.Stdout` directly, so tests can capture output through the cobra command. `Execute` maps errors to exit codes: 0 success, 130 picker cancelled, 1 otherwise (errors printed as `error: ...` to stderr; `errAlreadyReported` suppresses double printing).

All state lives under the gcloud config dir (`$CLOUDSDK_CONFIG` or OS default): gcloud's own `configurations/config_<name>`, `active_config`, `config_sentinel`, live `application_default_credentials.json`, plus gcloud-ctx's `gcloud-ctx/previous` and `gcloud-ctx/adc/<name>.json`. This means `CLOUDSDK_CONFIG` sandboxing isolates everything, including gcloud-ctx's own state.

## Hard invariants (from DESIGN.md)

- **stdout is machine-consumable data only** (lists, `-c`, `show` body). Status lines, hints, warnings, errors → stderr.
- **No network calls at switch time**; never implement OAuth flows (`refresh` shells out to real `gcloud`). Never read/write `credentials.db` / `access_tokens.db`.
- **Never shell out to `gcloud config set`** — write INI directly via `internal/gcloud`.
- INI round trips must preserve unknown sections/keys: always load-whole-file → mutate → save, through the package's shared `ini.LoadOptions` (inline `#`/`;` in values and Python-style continuation lines must survive).
- ADC files are credentials: mode 0600, atomic rename-into-place, never log contents.
- Ordering matters and is spec'd: ADC is written **before** the gcloud property in `impersonate`/`--clear`; switch stages the ADC before touching `active_config` and commits after. Partial-failure error messages have exact wording in DESIGN.md.
- Config dir resolution matches gcloud exactly — do **not** honor `XDG_CONFIG_HOME`.
- No automatic ADC snapshotting; `adc save` is the only way snapshots are created.

## Testing conventions

- Filesystem tests set `CLOUDSDK_CONFIG` to a `t.TempDir()` fixture tree and drive the cobra root command end-to-end; they must never touch the real `~/.config/gcloud`.
- Table-driven tests for pure logic; golden files for INI round trips and synthesized ADC JSON. Fixtures live next to the package that owns them (`internal/gcloud/testdata/`, `internal/adc/testdata/`), not in the root `testdata/`.
- Skip file-permission assertions on Windows; CI runs ubuntu/macos/windows.

## Release

GoReleaser (`.goreleaser.yaml`) on `v*` tags; CI includes a `goreleaser check` job. GitHub Actions are SHA-pinned — keep that convention when touching workflows. The repo also ships an Agent Skill at `skills/gcloud-ctx/SKILL.md` documenting safe agent usage of the built CLI.

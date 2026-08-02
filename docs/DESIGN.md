# gcloud-ctx Design

`gcloud-ctx` is a fast context switcher for the `gcloud` CLI, modeled after
[kubectx](https://github.com/ahmetb/kubectx). A **context** is:

1. a gcloud **named configuration** (`~/.config/gcloud/configurations/config_<name>`:
   account, project, region/zone, `auth/impersonate_service_account`, …), plus
2. an optional per-context **Application Default Credentials (ADC)** file managed by
   gcloud-ctx and installed to `application_default_credentials.json` on switch.

The differentiator over plain `gcloud config configurations activate`: switching a
context also switches **ADC** (what Terraform / client libraries / anything using
google-auth sees) and keeps **service account impersonation** consistent across both
the gcloud CLI (`auth/impersonate_service_account` property) and ADC
(`impersonated_service_account` credential JSON). These are two independent
mechanisms in gcloud; leaving them out of sync is a classic footgun.

## Non-goals

- Never read or write `credentials.db` / `access_tokens.db` (gcloud's own OAuth token
  stores, keyed by account email, shared across configurations).
- No OAuth/browser flows. Obtaining the base user ADC stays `gcloud auth
  application-default login`'s job.
- No network calls at switch time. Switching is pure local file manipulation, exactly
  like `gcloud config configurations activate`.

## Command surface (kubectx grammar)

```
gcloud-ctx                          List contexts (interactive fuzzy picker on a TTY)
gcloud-ctx <NAME>                   Switch to context NAME
gcloud-ctx -                        Switch to the previous context
gcloud-ctx -c, --current            Print current context name
gcloud-ctx -l, --long               List as a table: NAME, ACTIVE, ACCOUNT, PROJECT, IMPERSONATION, ADC
gcloud-ctx <NEW>=<OLD>              Rename OLD to NEW ('.' = current)
gcloud-ctx -d, --delete <NAME>...   Delete context(s) (refuses the active one, like gcloud)
gcloud-ctx -u, --unset              Activate gcloud's virtual NONE configuration

gcloud-ctx create <NAME> [--project P] [--account A] [--impersonate SA] [--no-activate]
gcloud-ctx show [NAME]              Show context details incl. impersonation and ADC status
gcloud-ctx impersonate <SA> [--delegates d1,d2] [--quota-project P] [--context NAME]
gcloud-ctx impersonate --clear [--context NAME]
gcloud-ctx adc save [NAME]          Snapshot the live ADC file into the store for NAME (default: current)
gcloud-ctx completion bash|zsh|fish|powershell
gcloud-ctx --version
```

Subcommand names (`create`, `show`, `impersonate`, `adc`, `completion`, and cobra's
own auto-generated `help`) shadow context names at the root level (a config literally
named `show` must be switched to via `gcloud-ctx show`… it can't; document this as a
known limitation — such names are unlikely given gcloud's name charset).

### Output conventions

- stdout is reserved for machine-consumable data: the list, `-c` output, `show` body.
- All status lines, hints, warnings, errors go to **stderr**. Errors are prefixed
  `error: `; exit code 1 on failure.
- Colors (active context: yellow + bold) only when stdout is a TTY; suppressed by
  `NO_COLOR` (any non-empty value, per no-color.org — color only, keep bold);
  `FORCE_COLOR` re-enables.
- Interactive picker: bare invocation + stdin/stdout both TTYs +
  `GCLOUD_CTX_IGNORE_FZF` unset → `github.com/ktr0731/go-fuzzyfinder` picker with a
  preview pane (account/project/impersonation). Cancel → silent exit, code 130.
  Non-TTY or env set → plain sorted list, one name per line.

## gcloud interop (facts verified against Cloud SDK 575 source)

- **Config dir resolution** (match `config.py:_GetGlobalConfigDir` exactly):
  `$CLOUDSDK_CONFIG` if set; else Windows: `%APPDATA%\gcloud` (fallback
  `%SystemDrive%\gcloud`); else `$HOME/.config/gcloud` via `os.UserHomeDir()`.
  **Do not honor `XDG_CONFIG_HOME`** — real gcloud ignores it.
- **Config name rules**: `^[a-z][-a-z0-9]*$`. The only reserved name is the literal
  `NONE` — a virtual, file-less, read-only configuration. Switching **to** `NONE` is
  legal (that's `--unset`); creating/renaming to it is not, and property writes while
  it's active must be refused.
- **Active config**: `active_config` file contains the bare name, **no trailing
  newline**. Effective active resolution order: `CLOUDSDK_ACTIVE_CONFIG_NAME` env var
  > `active_config` file (we have no `--configuration` flag). The env var may name a
  config with no backing file — treat as valid, not corruption.
- **Switch = write `active_config` + touch `config_sentinel`** (empty write; its
  mtime tells IDE plugins/credential helpers to re-read config). No locking needed —
  gcloud itself does a plain overwrite.
- **Delete/rename guard**: refuse when the target is the active config, checked
  against **both** the raw `active_config` file **and** the effective (env-var)
  active name — same double check as gcloud.
- **Config INI files**: parse/write with `gopkg.in/ini.v1`, always
  load-whole-file → mutate key → save-whole-file, so unknown sections/keys written by
  gcloud or other tools survive round trips (golden-file test required). An empty
  (0-byte) file is a valid, brand-new configuration. Relevant keys:
  `core.account`, `core.project`, `compute.region`, `compute.zone`,
  `auth.impersonate_service_account`. Every parse in the package goes through one
  shared `ini.LoadOptions` (`IgnoreInlineComment: true`,
  `AllowPythonMultilineValues: true`): a `#`/`;` inside a value must survive a
  round trip rather than being truncated there, and a configparser-style indented
  continuation line (written by gcloud or another tool) must parse instead of
  erroring out. Known non-goal: `ini.v1` unconditionally strips a value's
  surrounding double quotes on parse with no cheap way to tell `key = "v"` apart
  from `key = v` afterward, so that quoting is not preserved on round trip.
- **Never shell out to `gcloud config set`** — it refuses to write unrelated
  properties when the active account lacks cached credentials. Write INI directly.

## ADC management

Paths: live ADC = `<configdir>/application_default_credentials.json`. gcloud-ctx's
own state lives in `<configdir>/gcloud-ctx/`:

```
<configdir>/gcloud-ctx/previous          # previous context name (for `gcloud-ctx -`)
<configdir>/gcloud-ctx/adc/<name>.json   # stored ADC per context
```

Keeping state inside the gcloud config dir means `CLOUDSDK_CONFIG` sandboxing
(tests!) isolates everything, and state is naturally scoped per config dir.

### Switch semantics (explicit, no magic)

On `gcloud-ctx <NAME>`, ADC handling is staged and committed around
`active_config` rather than interleaved with it, so a late failure can be reported
precisely instead of leaving ADC and `active_config` in an unreported
inconsistent state:

1. Validate name; require `configurations/config_<NAME>` to exist (except `NONE`,
   which is always a legal target and never has a stored ADC — see below — so it
   naturally skips ADC handling entirely and never prints the no-stored-ADC hint).
2. Read the raw `active_config` value as OLD; if OLD ≠ NAME write OLD to `previous`.
3. If `gcloud-ctx/adc/<NAME>.json` exists, load and parse it. If it parses as one of
   the known ADC credential types, **stage** it: write a temp file in the same
   directory as the live ADC, mode 0600, without installing it yet. A staging
   failure (disk full, permissions) surfaces here, before `active_config` has been
   touched at all, so the switch itself is refused rather than half-completed.
4. Activate NAME: write `active_config` = NAME (exact bytes), touch
   `config_sentinel`. If this fails, remove the staged file (if any) and return the
   error — nothing was committed.
5. Commit, based on what step 3 found:
   - A staged file → rename it into place (resolving a live-ADC symlink target the
     same way `adc.Install` does). If this rename fails, `active_config` has
     already moved: report it explicitly as `switched to "NAME" but live ADC still
     belongs to the previous context: ...` rather than a generic error.
   - A stored ADC that didn't parse as a known type → the switch still completes;
     print a warning instead of an error (`stored ADC for "NAME" is unparseable;
     live ADC unchanged (re-create it with gcloud-ctx adc save)`) and leave the live
     ADC file untouched.
   - No stored ADC at all (and NAME isn't `NONE`) → print the hint
     (`ADC unchanged (no stored ADC for "NAME"; run 'gcloud-ctx adc save')`).
6. Warnings (stderr): `CLOUDSDK_ACTIVE_CONFIG_NAME` set → the switch is shadowed for
   this shell; `GOOGLE_APPLICATION_CREDENTIALS` set → live ADC file is ignored by
   libraries.
7. `Switched to context "NAME".` to stderr.

There is **no automatic ADC snapshotting** — `gcloud-ctx adc save` is the explicit
way to bind the current live ADC to a context. Predictability over magic.

ADC files are credentials: always mode 0600, atomic rename-into-place, never log
contents. `adc save` refuses if the live ADC file doesn't exist, doesn't parse as
JSON, or parses to a credential type gcloud-ctx doesn't recognize (i.e. not one of
the types gcloud itself writes) — a stored snapshot is guaranteed to later be
installable and, if wrapped, unwrappable.

### Impersonation

`gcloud-ctx impersonate SA@… [--delegates d1,d2] [--quota-project P] [--context NAME]`
(target context defaults to current; `NONE` refused; the target SA and every
delegate are validated against a strict service-account-email regex:
`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.gserviceaccount\.com$` — tight enough to reject
`/`, `:`, `?`, `#`, spaces, and newlines from ever reaching the INI file or an
impersonation URL):

1. Determine **base credentials**: the context's stored ADC if present, else the
   live ADC (with a stderr notice — `using current live ADC (account ...) as base
   credentials for context "NAME"` — since falling back to the live ADC means
   borrowing whatever is currently active, which may not be NAME itself, most
   notably with `impersonate --context OTHER`). If it's `impersonated_service_account`,
   unwrap `source_credentials`. Accepted base types: `authorized_user`,
   `service_account`, `external_account_authorized_user`. Anything else (or no ADC
   at all) → error telling the user to run `gcloud auth application-default login`
   first.
2. Synthesize the impersonated ADC JSON — exactly what gcloud writes:

```json
{
  "delegates": [],
  "service_account_impersonation_url": "https://iamcredentials.{universe_domain}/v1/projects/-/serviceAccounts/{SA}:generateAccessToken",
  "source_credentials": { …base credential object… },
  "type": "impersonated_service_account"
}
```

   `universe_domain` from the base credential (default `googleapis.com`); delegates
   from `--delegates` (order = chain toward target); `quota_project_id` top-level if
   `--quota-project` given. Serialize with sorted keys, 2-space indent (match
   gcloud's `json.dumps(sort_keys=True, indent=2)`).
3. **ADC moves first**: save to `gcloud-ctx/adc/<name>.json`, then — if the context
   is the raw-active one — install it live too (atomic, 0600). Only once both of
   those succeed is `auth.impersonate_service_account` written, as a single
   gcloud-native comma chain with delegates first and the target last
   (`d1,d2,SA`, matching gcloud's own `ParseImpersonationAccounts` encoding — a
   property with no delegates is just the bare target SA). Writing the property
   last means a failure there is reported as `ADC updated for "NAME" but the
   gcloud property was not set: ...` — the caller learns ADC already moved even
   though the gcloud-native switch didn't. `show` and the `-l` table split the raw
   property back into target + delegates for display (inverse of the join).
4. `--clear`: if the stored ADC is `impersonated_service_account`, unwrap it back to
   `source_credentials`, save it, and install live if the context is raw-active.
   Independently of that stored-snapshot path, if NAME *is* the raw-active context,
   the live ADC file is read directly and unwrapped too if it's
   `impersonated_service_account` — this covers the live ADC actually impersonating
   even when there's no stored snapshot reflecting that (or gcloud-ctx's own store
   doesn't know about it); failures on this best-effort path are a warning, not a
   hard error, since the INI property still needs to be cleared regardless of what
   the live ADC file contains. Only after ADC handling completes is
   `auth.impersonate_service_account` deleted (drop the `[auth]` section if it
   becomes empty) — again ADC-then-property ordering, with a matching `ADC updated
   for "NAME" but the gcloud property was not cleared: ...` error if that last step
   fails.

Note: `roles/iam.serviceAccountTokenCreator` on the target SA is required at token
time; gcloud-ctx does not preflight (no network at switch time) — `show` displays the
target so failures are diagnosable.

### Other commands

- `create`: validate name, refuse existing/`NONE`; write INI directly with the given
  properties; activate by default (mirrors `gcloud config configurations create`),
  `--no-activate` to skip. Does not touch ADC (hint at `impersonate`/`adc save`).
- `rename NEW=OLD` (`.` for OLD means the current effective context): refuse if NEW
  exists/invalid, or OLD doesn't exist/is `NONE`. Unlike gcloud's own
  `configurations rename`, renaming the *active* configuration is allowed rather
  than refused: `os.Rename` the config file first, then — if OLD was the raw
  `active_config` value — rewrite `active_config` to NEW and touch
  `config_sentinel`, so the active configuration stays coherent throughout; the
  confirmation message says so explicitly (`... renamed to ... (active).`). If
  `CLOUDSDK_ACTIVE_CONFIG_NAME` still names OLD, warn that this shell's active
  context won't follow the rename until the env var is updated (rename has no way
  to fix up the calling shell's environment). Also move `adc/<old>.json` if
  present, and rewrite `previous` if it pointed at OLD.
- `delete`: refuse active (both checks) and `NONE`; remove config file and
  `adc/<name>.json`; clear `previous` if it pointed at the deleted name. No
  confirmation prompt (kubectx behavior); message per deleted name to stderr.
- `show [NAME]`: account, project, region, zone, impersonation target and delegates
  (split from the INI property's comma chain), stored-ADC type + impersonation
  target parsed from the URL, whether live ADC == stored ADC (byte compare), plus
  the env-var warnings from switch.

## Code layout

```
main.go                 # thin: version vars (ldflags), cli.Execute()
internal/cli/           # cobra commands; root implements the kubectx grammar
internal/gcloud/        # config dir resolution, named-config store (list/active/
                        # activate/create/rename/delete), INI property access
internal/adc/           # ADC file model: read/type-sniff/synthesize impersonated/
                        # unwrap/atomic 0600 install
internal/store/         # gcloud-ctx state: previous file, per-context ADC store
internal/ui/            # color handling (NO_COLOR/FORCE_COLOR/TTY), fuzzy picker
```

Fixture INI/ADC files and golden outputs live next to the package that owns them
(`internal/gcloud/testdata/`, `internal/adc/testdata/`), not in a shared root
`testdata/` — there is no root `testdata/` directory.

Module `github.com/sakajunquality/gcloud-ctx`, `go 1.25` (no `toolchain` directive).
Dependencies: `spf13/cobra`, `gopkg.in/ini.v1`, `ktr0731/go-fuzzyfinder` (pinned
tags). No Viper, no mocking frameworks, no bubbletea.

Shell completion: cobra-generated; root `ValidArgsFunction` completes context names
by reading the config dir (shared code path with `list`).

## Testing

- Table-driven unit tests: name validation, config-dir resolution (env permutations),
  ADC type sniffing/synthesis/unwrap (golden JSON), impersonation URL build/parse.
- Filesystem tests: `t.TempDir()` as `CLOUDSDK_CONFIG` with fixture trees; full
  switch/rename/delete/impersonate/adc-save flows through the cobra root command;
  assert file contents, 0600 modes (skip perm asserts on Windows), sentinel mtime
  bump, `previous` bookkeeping.
- Golden-file round trip: INI with unknown sections/comments survives a property
  write.
- CI runs `go test -race ./...` on ubuntu/macos/windows.

## Release & OSS

- Apache-2.0 (gcloud-ecosystem convention, patent grant; same as kubectx).
- GoReleaser: darwin/linux/windows × amd64/arm64, `CGO_ENABLED=0`,
  `-s -w -X main.version={{.Version}} …`, tar.gz (zip on Windows), checksums,
  Homebrew *cask* (`homebrew_casks`, not the deprecated `brews`) pushed to
  `sakajunquality/homebrew-tap` (needs a PAT secret; the tap doesn't exist until
  the first tagged release publishes it, so README install steers people to
  `go install`/GitHub Releases in the meantime).
- GitHub Actions: `ci.yml` (test matrix, golangci-lint with a version pinned to
  match `golangci-lint-action`'s major, and a `goreleaser check` job),
  `release.yml` (goreleaser on `v*` tags, `contents: write`, goreleaser version
  pinned to a `~> vX.Y` constraint rather than `latest`).
- README: pitch → install (`go install`, GitHub Releases binaries; Homebrew tap
  noted as available once the first tag ships) → usage block → "how it works"
  (local file ops only; what gets touched and what never does) → impersonation
  walkthrough → env vars (`CLOUDSDK_CONFIG`, `CLOUDSDK_ACTIVE_CONFIG_NAME`,
  `GOOGLE_APPLICATION_CREDENTIALS`, `GCLOUD_CTX_IGNORE_FZF`, `NO_COLOR`,
  `FORCE_COLOR`) → comparison with prior art (verified against each repo, not
  assumed) → license. Suggest `alias gctx=gcloud-ctx`.

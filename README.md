# gcloud-ctx

[![CI](https://github.com/sakajunquality/gcloud-ctx/actions/workflows/ci.yml/badge.svg)](https://github.com/sakajunquality/gcloud-ctx/actions/workflows/ci.yml)
[![Release](https://github.com/sakajunquality/gcloud-ctx/actions/workflows/release.yml/badge.svg)](https://github.com/sakajunquality/gcloud-ctx/actions/workflows/release.yml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

`gcloud-ctx` is a fast, [kubectx](https://github.com/ahmetb/kubectx)-style context
switcher for the `gcloud` CLI. A context is a named `gcloud` configuration (account,
project, region/zone, impersonation) *plus* an optional per-context Application
Default Credentials (ADC) snapshot, so switching contexts also switches what
Terraform, client libraries, and anything else built on `google-auth` sees — and
keeps service account impersonation consistent across both the `gcloud` CLI and ADC,
instead of leaving them silently out of sync.

## Install

Via `go install`:

```
go install github.com/sakajunquality/gcloud-ctx@latest
```

Or download a prebuilt binary for your OS/arch from [GitHub
Releases](https://github.com/sakajunquality/gcloud-ctx/releases).

Consider aliasing it, kubectx-style:

```
alias gctx=gcloud-ctx
```

## Usage

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
gcloud-ctx env [NAME]               Print exports that pin NAME for one shell only (see below)
gcloud-ctx env --unset              Print the matching unset lines
gcloud-ctx refresh [NAME]           Re-run ADC login and rebuild every same-account snapshot
gcloud-ctx completion bash|zsh|fish|powershell
gcloud-ctx --version
```

The interactive picker (bare `gcloud-ctx` on a TTY) shows a preview pane with
account/project/impersonation for the highlighted context; cancel with `Esc`/`Ctrl-C`
to exit without switching. Set `GCLOUD_CTX_IGNORE_FZF` to always fall back to a plain
sorted list instead (useful in scripts piping through a pager).

> **Known limitation:** subcommand names (`create`, `show`, `impersonate`, `adc`,
> `env`, `refresh`, `completion`, `help`) shadow context names at the root level — a context
> literally named `show` can't be switched to via `gcloud-ctx show`. This is unlikely
> in practice given `gcloud`'s config name charset (`^[a-z][-a-z0-9]*$`), but it's a
> known tradeoff of reusing the kubectx grammar.

## Re-authentication

ADC snapshots are file copies of refresh tokens. Tokens don't normally expire,
but Workspace re-auth policies, revocation, or a password change kill them —
and every snapshot copied from that token dies at once. One command recovers
everything:

```sh
gcloud-ctx refresh            # or: gcloud-ctx refresh work
```

It runs `gcloud auth application-default login` (browser), updates the
context's snapshot — **preserving impersonation targets/delegates/quota
projects** for impersonated snapshots — rebuilds every other context's
snapshot derived from the same account, and leaves the live ADC matching the
active context. Other accounts' snapshots are untouched.

The gcloud CLI's own login is a separate per-account token store; if `gcloud`
itself says `Reauthentication required`, also run `gcloud auth login` (once
per account — all contexts referencing that account share it).

## Per-shell contexts (multiple agents / terminals)

`gcloud-ctx <NAME>` switches machine-global state (`active_config` and the live
ADC file), which is what you want interactively — but several shells, terminal
tabs, or AI coding agents on one machine can't each hold a different context
that way. `gcloud-ctx env` pins a context to **one shell only**, using gcloud's
own per-process override plus the context's stored ADC snapshot, without
touching any global file:

```sh
eval "$(gcloud-ctx env prod)"
# exports for this shell and its children only:
#   CLOUDSDK_ACTIVE_CONFIG_NAME=prod            → gcloud CLI uses "prod"
#   GOOGLE_APPLICATION_CREDENTIALS=…/adc/prod.json → ADC consumers use "prod"'s snapshot
```

Undo with `eval "$(gcloud-ctx env --unset)"`. The `GOOGLE_APPLICATION_CREDENTIALS`
line is emitted only when the context has a stored snapshot (`gcloud-ctx adc save`);
otherwise the variable is unset so it can't point at a stale snapshot. While these
variables are set, `gcloud-ctx <NAME>` switches in that shell have no visible
effect — the tool warns about exactly this on stderr.

## How it works

`gcloud-ctx` only ever does local file manipulation — no network calls at switch
time, exactly like `gcloud config configurations activate`.

**Files it reads and writes**, all under your `gcloud` config directory
(`$CLOUDSDK_CONFIG`, or the OS default `gcloud` uses):

- `configurations/config_<name>` — the named configuration's INI properties
  (account, project, region, zone, impersonation), parsed and rewritten in place so
  unrelated keys survive.
- `active_config` — which configuration is active. Renaming the *active* context
  (including `gcloud-ctx NEW=.`) is supported: `active_config` is rewritten to
  follow the new name, so the switch stays coherent.
- `config_sentinel` — touched (empty write) on every mutation — switch, create,
  delete, rename, or an impersonation/property change — so IDE plugins and
  credential helpers know to re-read config, exactly like `gcloud` itself.
- `application_default_credentials.json` — the live ADC file, atomically replaced
  (temp file + rename, mode `0600`) when the target context has a stored ADC. The
  virtual `NONE` configuration (`gcloud-ctx --unset`) never has a stored ADC and
  gcloud-ctx never writes one for it — switching to `NONE` always leaves the live
  ADC file untouched.
- `gcloud-ctx/` — gcloud-ctx's own state: `gcloud-ctx/previous` (for `gcloud-ctx -`)
  and `gcloud-ctx/adc/<name>.json` (one stored ADC snapshot per context). Living
  inside the `gcloud` config dir means test/sandbox isolation via `$CLOUDSDK_CONFIG`
  covers gcloud-ctx's state too.

**Files it never touches:** `credentials.db`, `access_tokens.db`, or any other
`gcloud`-managed OAuth token store. Those are keyed by account email and shared
across configurations — obtaining or refreshing the underlying credentials stays
`gcloud auth login` / `gcloud auth application-default login`'s job. gcloud-ctx never
performs an OAuth/browser flow.

There is no automatic ADC snapshotting: `gcloud-ctx adc save` is the explicit way to
bind the current live ADC to a context. Predictability over magic. `adc save`
validates the live ADC file first — it must parse as JSON and have one of the
credential types gcloud itself writes — and refuses rather than storing something
that can't later be installed or unwrapped.

## Impersonation walkthrough

Suppose you want context `prod` to use impersonation of a deploy service account.

1. Grant yourself (or the base credential) the right to impersonate it:

   ```
   gcloud iam service-accounts add-iam-policy-binding \
     deploy@my-project.iam.gserviceaccount.com \
     --member="user:you@example.com" \
     --role="roles/iam.serviceAccountTokenCreator"
   ```

2. Point the context at it:

   ```
   gcloud-ctx impersonate deploy@my-project.iam.gserviceaccount.com --context prod
   ```

   `prod`'s stored ADC — or the live ADC if none is stored yet — is unwrapped (if
   necessary) and used as `source_credentials` to synthesize an
   `impersonated_service_account` ADC JSON:

   ```json
   {
     "delegates": [],
     "service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/deploy@my-project.iam.gserviceaccount.com:generateAccessToken",
     "source_credentials": { "...": "..." },
     "type": "impersonated_service_account"
   }
   ```

   That JSON is saved to `gcloud-ctx/adc/prod.json` and, if `prod` is the active
   context, installed as the live ADC too — **before** `auth/impersonate_service_account`
   is written to `config_prod`, so a failure at that last step is reported precisely
   (`ADC updated for "prod" but the gcloud property was not set: ...`) instead of
   leaving ADC and the gcloud property silently disagreeing. Once both succeed,
   `gcloud` CLI calls and ADC-based libraries agree on the same impersonated
   identity.

   If `--context` names a context other than the one actually active, and that
   context has no stored ADC of its own, gcloud-ctx borrows the *live* ADC as the
   base credential and says so on stderr (`using current live ADC (account ...) as
   base credentials for context "prod"`) — worth noticing, since it means the
   synthesized credential is based on whoever is currently logged in, not
   necessarily `prod`'s own identity.

   A delegation chain toward the target is `--delegates d1,d2`: gcloud-ctx records
   it in `auth/impersonate_service_account` the same way `gcloud` itself does — one
   comma-joined chain, delegates first, target last (`d1,d2,deploy@my-project.iam.gserviceaccount.com`)
   — and lists the same delegates (in order) in the synthesized ADC JSON's
   `delegates` array.

3. `gcloud-ctx show prod` reports the impersonation target and delegates (parsed
   back out of that comma chain), and whether the live ADC matches the stored one,
   so a `PERMISSION_DENIED` at token-generation time (missing the IAM binding from
   step 1) is easy to diagnose.

4. `gcloud-ctx impersonate --clear --context prod` undoes it: unwraps the stored
   ADC back to its `source_credentials` (installing live too if `prod` is active —
   and if `prod` *is* the live active context, its live ADC file is unwrapped
   directly even if gcloud-ctx's own store has no record of the impersonation),
   then removes `auth/impersonate_service_account`. As with `impersonate` itself,
   ADC moves first; a failure clearing the property is reported as `ADC updated for
   "prod" but the gcloud property was not cleared: ...`.

## Environment variables

| Variable                       | Effect                                                                 |
| ------------------------------ | ----------------------------------------------------------------------- |
| `CLOUDSDK_CONFIG`               | Overrides the `gcloud` config directory (same as `gcloud` itself).      |
| `CLOUDSDK_ACTIVE_CONFIG_NAME`   | Overrides the active configuration for the current shell; gcloud-ctx warns that a switch is shadowed while this is set. |
| `GOOGLE_APPLICATION_CREDENTIALS` | If set, ADC libraries read this file instead of the live ADC gcloud-ctx manages; gcloud-ctx only reads this to warn you it's set (on `show`/switch), it never reads or writes the file itself. |
| `GCLOUD_CTX_IGNORE_FZF`         | Any non-empty value disables the interactive fuzzy picker; falls back to a plain sorted list. |
| `NO_COLOR`                      | Any non-empty value disables color output (per [no-color.org](https://no-color.org/)); bold is kept. |
| `FORCE_COLOR`                   | Re-enables color even when stdout isn't a TTY.                          |

## Comparison with prior art

Several other tools in this space *do* switch ADC — the differentiator isn't "we're
the only one who touches ADC," it's keeping **impersonation** in sync across both
places gcloud tracks it (the `auth/impersonate_service_account` gcloud property and
the ADC JSON's `impersonated_service_account` wrapper) as part of the same kubectx-
faithful switch, rather than as a separate subcommand-driven flow:

| Tool | Grammar | Switches gcloud config | Switches ADC | Impersonation kept in sync |
| --- | --- | --- | --- | --- |
| [`ogerbron/gcloudctx`](https://github.com/ogerbron/gcloudctx) | kubectx grammar (bash port) | Yes | No | No |
| [`uhinze/gconf`](https://github.com/uhinze/gconf) | kubectx-inspired (bash); list/switch/`-` only, no rename or delete | Yes | No | No |
| [`agadelshin/gcloudctx`](https://github.com/agadelshin/gcloudctx) | kubectx-ish (Go); its own README lists colorized output, completions, and interactive mode as still TODO | Yes | No | No |
| [`xgourmandin/gcloud-switch`](https://github.com/xgourmandin/gcloud-switch) | subcommand (`add`/`list`/`switch`/`edit`/`current`) | Yes | Yes — reuses valid credentials, re-authenticates when needed | Optional per-config SA flag; not documented as synced back to ADC |
| [`k0wl0n/gctx`](https://github.com/k0wl0n/gctx) | subcommand (Cobra CLI) | Yes | Yes — a separate ADC snapshot per account, swapped on switch | No |
| [`tjirsch/gcloud-switch`](https://github.com/tjirsch/gcloud-switch) | TUI + scripting subcommands (Rust) | Yes (user credentials) | Yes | No |
| **gcloud-ctx** | kubectx grammar (`gcloud-ctx <NAME>`, `-`, `-c`, `-l`, ...) | Yes | Yes | **Yes** — one switch keeps `auth/impersonate_service_account` and the ADC JSON in agreement |

(Table current as of 2026-08; verified each repo exists and read its README before
describing it here. Two tools previously listed here — under the names `gcloudctx`
at `newkozlukov/gcloudctx` and `gcloud-switch` at `wshihadeh/gcloud-switch` — no
longer resolve to a repository and have been dropped.)

## AI agents (Agent Skill)

The repo ships an [Agent Skill](skills/gcloud-ctx/SKILL.md) that teaches AI
coding agents (Claude Code and other SKILL.md-compatible tools) how to operate
gcloud-ctx safely: non-interactive usage, credentials hygiene, impersonation
recipes, and how to sandbox experiments with `CLOUDSDK_CONFIG`.

Install for Claude Code (per project or globally):

```sh
mkdir -p .claude/skills/gcloud-ctx      # or ~/.claude/skills/gcloud-ctx
curl -fsSL https://raw.githubusercontent.com/sakajunquality/gcloud-ctx/main/skills/gcloud-ctx/SKILL.md \
  -o .claude/skills/gcloud-ctx/SKILL.md
```

## License

Apache-2.0, see [LICENSE](LICENSE).

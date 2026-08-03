---
name: gcloud-ctx
description: >-
  Switch Google Cloud contexts with the gcloud-ctx CLI: gcloud named
  configurations, Application Default Credentials (ADC), and service account
  impersonation, all kept in sync in one switch. Use when the user asks to
  switch GCP projects/accounts/contexts, set up or clear service account
  impersonation, bind ADC to a context, or diagnose "gcloud and Terraform are
  using different identities" problems.
license: Apache-2.0
---

# Operating gcloud-ctx

`gcloud-ctx` switches between gcloud **contexts**. A context is a gcloud named
configuration (account, project, region/zone, impersonation property) plus an
optional stored ADC snapshot. Switching a context updates what `gcloud` uses
AND what ADC consumers (Terraform, client libraries) use — the two are
independent mechanisms in gcloud and this tool exists to keep them in sync.

Requires the `gcloud-ctx` binary on PATH. Verify with `gcloud-ctx --version`.

## Agent essentials

- **Non-interactive by design**: when stdout is not a TTY (your shell), bare
  `gcloud-ctx` prints a plain sorted list — no picker, no colors. If you ever
  need to force that in a TTY-like environment, set `GCLOUD_CTX_IGNORE_FZF=1`.
- **stdout = data, stderr = messages.** Parse stdout only. Exit code 0 on
  success, 1 on failure.
- **Credentials hygiene**: `application_default_credentials.json` and the
  snapshots under `<configdir>/gcloud-ctx/adc/` contain live refresh tokens.
  Never print their contents, never commit them, never hand-edit them. Use
  `gcloud-ctx show` to inspect state instead of reading the files.
- **Before switching**, capture the current context (`gcloud-ctx -c`) so you
  can restore it with `gcloud-ctx -` (previous) or an explicit name when your
  task is done. Restore the user's original context unless they asked to stay.
- **Shadowing env vars**: if `CLOUDSDK_ACTIVE_CONFIG_NAME` is set, switches
  don't affect the current shell; if `GOOGLE_APPLICATION_CREDENTIALS` is set,
  the ADC file is ignored by libraries. gcloud-ctx warns on stderr in both
  cases — surface those warnings to the user instead of ignoring them.

## Commands

```
gcloud-ctx                          List context names (plain when piped)
gcloud-ctx -l                       Table: NAME ACTIVE ACCOUNT PROJECT IMPERSONATION ADC
gcloud-ctx -c                       Print current context name
gcloud-ctx <NAME>                   Switch (gcloud config + stored ADC if any)
gcloud-ctx -                        Switch to previous context
gcloud-ctx <NEW>=<OLD>              Rename ('.' = current; renaming the active context works)
gcloud-ctx -d <NAME>...             Delete (refuses the active context)
gcloud-ctx -u                       Activate gcloud's virtual NONE configuration
gcloud-ctx create <NAME> [--project P] [--account A] [--impersonate SA] [--no-activate]
gcloud-ctx show [NAME]              Details: props, impersonation target/delegates, ADC status
gcloud-ctx impersonate <SA> [--delegates d1,d2] [--quota-project P] [--context NAME]
gcloud-ctx impersonate --clear [--context NAME]
gcloud-ctx adc save [NAME]          Bind the current live ADC file to a context
gcloud-ctx env [NAME]               Print POSIX exports pinning NAME for one shell only
gcloud-ctx env --unset              Print the matching unset lines
gcloud-ctx refresh [NAME]           Re-run ADC login, rebuild same-account snapshots
```

## Recipes

**Create a context and bind the current credentials to it:**

```sh
gcloud-ctx create staging --project my-staging-proj --account user@example.com
gcloud-ctx adc save    # now switching to "staging" also installs this ADC
```

Without `adc save`, switching changes only the gcloud configuration and prints
`ADC unchanged (...)` on stderr — that is expected, not an error.

**Set up service account impersonation (both mechanisms at once):**

```sh
gcloud-ctx impersonate deployer@proj.iam.gserviceaccount.com
```

This writes the gcloud property (`auth/impersonate_service_account`, encoded
`delegate1,delegate2,TARGET` when delegates are used) and synthesizes the
`impersonated_service_account` ADC JSON locally from the existing user ADC —
no browser flow. The caller needs `roles/iam.serviceAccountTokenCreator` on
the target SA; gcloud-ctx does not preflight that, so a later 403 from an API
means the grant is missing, not that the switch failed.

Note: `create --impersonate SA` sets only the gcloud property. Run
`gcloud-ctx impersonate SA` afterwards to make ADC match.

**Use a context in THIS shell only (multi-agent / parallel sessions):**

`gcloud-ctx <NAME>` switches machine-global state and would affect every other
shell and agent on the machine. When you are one of several concurrent
sessions, or you must not disturb the user's global context, pin per-shell
instead:

```sh
eval "$(gcloud-ctx env prod)"     # this shell + children only; no global file touched
# ... do the work ...
eval "$(gcloud-ctx env --unset)"  # back to global behavior
```

This sets `CLOUDSDK_ACTIVE_CONFIG_NAME` (gcloud CLI side) and
`GOOGLE_APPLICATION_CREDENTIALS` (ADC-consumer side, pointing at the context's
stored snapshot). It requires the context to have a stored ADC snapshot for
the ADC half — without one, only the gcloud side is pinned and a stderr hint
says so. Prefer this over global switching whenever the user didn't explicitly
ask to change the machine-wide context. Note that while these variables are
set, global `gcloud-ctx <NAME>` switches have no effect in this shell.

**Recover from expired/revoked credentials (`invalid_grant`, "Token has been
expired or revoked", Workspace re-auth policies):**

```sh
gcloud-ctx refresh    # interactive: opens a browser via gcloud — needs the user present
```

This re-runs ADC login and rebuilds every snapshot derived from the same
account (impersonation targets are preserved). It is interactive: do not run
it unattended; tell the user it will open a browser. If plain `gcloud`
commands also fail with `Reauthentication required`, additionally run
`gcloud auth login` (once per account). A 403 during impersonation is a
permissions problem (`roles/iam.serviceAccountTokenCreator`), not expiry —
don't refresh for that.

**Diagnose identity mismatches:**

```sh
gcloud-ctx show          # Impersonation (gcloud side) vs Stored/Live ADC (library side)
```

`Live ADC: differs from stored` or an impersonation target with
`Stored ADC: none` are the classic desync signatures.

**Verify a switch took effect for gcloud itself:**

```sh
gcloud config configurations list   # is_active should match gcloud-ctx -c
```

## Safety rules

- Prefer `gcloud-ctx` over editing anything in `~/.config/gcloud` directly,
  and over `gcloud config configurations activate` (which does not switch ADC).
- Deleting a context also deletes its stored ADC snapshot. Confirm with the
  user before `-d` unless they named the context explicitly.
- For experiments or tests, sandbox everything with
  `CLOUDSDK_CONFIG=$(mktemp -d)` — gcloud, gcloud-ctx, and all state
  (including snapshots) follow it, and the user's real config is untouched.

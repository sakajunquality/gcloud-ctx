# Contributing to gcloud-ctx

Thanks for considering a contribution.

## Build

```
go build ./...
```

## Test

```
go test -race ./...
```

Filesystem tests point `CLOUDSDK_CONFIG` at a `t.TempDir()`, so they never touch
your real `~/.config/gcloud`. Please keep it that way in new tests.

## Lint

```
golangci-lint run
```

CI runs the same config (`.golangci.yml`) via `golangci-lint-action`, plus
`gofmt`/`goimports` formatting checks. Run `gofmt -l .` and `goimports -l .`
before pushing if you don't have `golangci-lint` installed locally.

## Pull requests

- Keep PRs focused on one change; explain the "why" in the description.
- Add or update table-driven tests for behavior changes, and a golden-file
  fixture under `testdata/` if you touch INI or ADC parsing/serialization.
- Update `docs/DESIGN.md` and `README.md` if you change command surface or
  file/env-var semantics.
- Make sure `go build ./...`, `go test -race ./...`, and `golangci-lint run`
  are clean before requesting review.

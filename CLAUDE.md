# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go client library for **familio.org** (an unofficial integration — no public
write API exists; endpoints were reverse-engineered from the tree editor). It
was extracted from
[terraform-provider-familio](https://github.com/dmalch/terraform-provider-familio)'s
`internal/familio` package so the same HTTP layer is reusable from CLI tools,
migration scripts, and other projects — not just the Terraform provider.

`API.md` is the source of truth for the familio.org HTTP surface — read it
before touching the client. It documents the reverse-engineered endpoints,
request/response shapes, and the auth model.

## Layout

- Root package `familio` (flat — every `*.go` is `package familio`). The
  domain is event-centric: persons, marriages, and life facts all share
  `Event` and the `DateRange` date model, so the client is one cohesive
  package rather than per-resource subpackages.
- `cmd/familio/` — a read-only CLI façade over the library (`whoami`,
  `person get`, `settlement get`, `settlement persons`, `sources list`).
- `examples/getperson/` — a minimal runnable usage example.

## Commands

```bash
make build            # go build ./...
make vet              # go vet ./...
make test             # go test ./...  (unit; no network)
make lint             # golangci-lint run ./...
make check            # build + vet + lint + test (CI parity)
make test-acceptance  # FAMILIO_NETWORK_TEST=1 — live read-only decode test
```

CI (`.github/workflows/ci.yaml`) runs build / test / vet / lint as four
parallel jobs on push to `main` and PRs.

## Auth is two-layer (the non-obvious part)

The `t` session cookie alone is **rejected** by the authed API (401).
familio's Next.js SSR embeds a short-lived **JWT bearer** in the page's
`__NEXT_DATA__`. So the client, from the `t` cookie, scrapes an HTML page for
`"token":"eyJ..."` (`auth.go`), caches it until ~5 min before its JWT `exp`,
and sends it as `Authorization: Bearer` on `/api/v2/*` calls. The JWT's `uuid`
claim is the account id, used as `?owner=` on creates and surfaced via
`Client.AccountUUID`. The public settlement-persons read needs neither cookie
nor bearer.

Cookies come from `Options.Cookies`; build them with `CookiesFromHeader`
(raw DevTools header), `CookieFromSessionToken` (bare `t` value), or
`CookiesFromBrowser` (logged-in browser via sweetcookie).

## Conventions

- Russian-language genealogy domain: user-facing names/data are often Cyrillic;
  keep it.
- Lint config (`.golangci.yml`) is strict and opt-in (`default: none` + an
  explicit enable list including `errcheck`, `errorlint`, `bodyclose`, `noctx`,
  `forcetypeassert`, `godot`). `godot` requires comment sentences to end with a
  period.
- Errors from the client are wrapped with `%w`; `ErrNotLoggedIn` is returned
  (via `CheckRedirect`) when a request bounces to a login path. `ErrNotFound`
  and `ErrAccessDenied` map 404/403.
- Tests use plain `go test` with `github.com/onsi/gomega` matchers (no Ginkgo).

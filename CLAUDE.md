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
- `cmd/familio/` — a CLI façade over the library (`whoami`, `person get`,
  `tree`, `graph`, `settlement get`, `settlement persons`, `sources list`,
  `history list`/`history filters`, `matches list`/`matches filters`,
  `tags list`/`tags person`/`tags by-persons`/`tags colors`, plus the
  `marriage create/delete`, `person set-biography`,
  `matches confirm/reject/undecide` and `tags create/update/delete/assign/unassign`
  writes). The `matches` mutations and `tags delete`/`assign`/`unassign` prompt
  `[y/N]` on stderr unless `-yes` is given; nothing else in the CLI prompts.
  CLI tests point the binary at an `httptest` server via `FAMILIO_BASE_URL`
  (`serveAPI` in `servertest_test.go`); it is env-only, not a flag.
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

## Auth (the non-obvious part)

The `t` cookie sent **as a cookie** is rejected (401); the authed API wants
`Authorization: Bearer <JWT>` and there is no mint endpoint. The trick is that
`t`'s *value* is itself a JWT, so `auth.go` sends it directly when it parses and
is not near expiry — and otherwise falls back to fetching an HTML page and
scraping `"token":"eyJ..."` out of `__NEXT_DATA__`. Either way the token is
cached until ~5 min before its `exp`. The JWT's `uuid` claim is the account id,
used as `?owner=` on creates and surfaced via `Client.AccountUUID`. The public
settlement-persons read needs neither cookie nor bearer.

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
- Errors: every response `>= 400` is an `*APIError` (method, path, status, body)
  that **wraps** the sentinel for its status, so `errors.Is` and `errors.As` both
  work. Sentinels: `ErrNotFound` 404, `ErrNotLoggedIn` 401 (or a `CheckRedirect`
  bounce to a login path), `ErrAccessDenied` 403, `ErrConflict` 409 (stale
  `X-Base-Version`). Other errors are wrapped with `%w`.
- Tests use plain `go test` with `github.com/onsi/gomega` matchers (no Ginkgo).
  Shared helpers are in `helpers_test.go`: `authedTestServer` (serves the token
  page on `/`), `newTestClient`, `newLiveClient`, `asMap`/`asSlice`. Fixtures are
  trimmed **real** responses — don't invent wire shapes.
- Since v1, semver covers **package `familio`'s exported identifiers only**.
  `cmd/familio` is best-effort — its commands, flags and JSON output may break in a
  minor release (note each in the changelog), because coupling them would let a flag
  rename force a `/v2` module path on library importers. `FAMILIO_BASE_URL` is the
  exception: it forwards to the exported `Options.BaseURL`, so it is stable. An
  *upstream* familio break is a patch release. See the README's Stability section and
  `CONTRIBUTING.md` for the release flow.

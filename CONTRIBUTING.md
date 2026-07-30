# Contributing

## The gate

```bash
make check            # build + vet + lint + test — exactly what CI runs
```

CI (`.github/workflows/ci.yaml`) runs build / test / vet / lint as four parallel
jobs on pushes to `main` and on pull requests. `make check` is the local
equivalent, so a green `make check` should mean a green CI.

Individual targets: `make build`, `make vet`, `make lint` (golangci-lint),
`make test` (unit, no network).

## Tests

Unit tests use plain `go test` with [gomega](https://github.com/onsi/gomega)
matchers — no Ginkgo. Endpoint tests stand up an `httptest` server and assert on
the **request** (method, path, headers, body) as much as the decode, because the
request shape is the part familio is fussy about: `application/ld+json` on authed
writes, `X-Base-Version` as a header rather than a body field, bare JSON arrays
on the `*-by-ids` and person↔tag endpoints.

Shared helpers live in `helpers_test.go`: `authedTestServer` (serves the
`__NEXT_DATA__` token page on `/`, delegates the rest to your handler),
`newTestClient`, `newLiveClient`, and `asMap`/`asSlice` for walking a decoded
request body. In `cmd/familio`, `serveAPI` does the same and points the CLI at
the fake server via `FAMILIO_BASE_URL`.

Fixtures should be **trimmed real responses**, not invented shapes. A fixture
that never came off the wire proves nothing about a reverse-engineered API.

## Live tests

The `*_network_test.go` tests hit production familio.org. They self-skip unless
`FAMILIO_NETWORK_TEST=1`, and CI never runs them.

```bash
make test-acceptance   # FAMILIO_NETWORK_TEST=1 go test -v -count=1 ./...
```

Credentials come from `FAMILIO_COOKIES`, `FAMILIO_SESSION`, or
`FAMILIO_BROWSER`, same precedence as the CLI.

**Run these before releasing anything that touches endpoints or wire types** —
they are the only check that upstream still answers the way `API.md` says.

The live tests here are deliberately **read-only**. Writes (create/update/delete)
mutate the real account, so they are covered by the `httptest` suite instead. If
you must verify a write against production, use a disposable test person and
delete it afterwards — and see `API.md` › Reverse-engineering method, which
documents that flow.

## Endpoint changes

`API.md` is the source of truth for familio's HTTP surface. When you add or
change an endpoint, update it in the same commit: the request/response shape, the
auth requirement, and anything surprising (a status code, an asymmetric write, a
PHP-ism like an empty map serialized as `[]`). A future reader should not have to
re-derive it from a browser session.

## Releasing

1. Bump `Version` in `client.go` (it feeds the default `User-Agent`).
2. Add a `CHANGELOG.md` entry in the existing style (a `### NEW` section for
   library surface, `### CLI` for commands).
3. `make check`, then `make test-acceptance` with a live session.
4. Commit, tag `vX.Y.Z`, push the tag. Go modules serve from the tag; there is no
   build artifact to publish.
5. For a release consumed by
   [terraform-provider-familio](https://github.com/dmalch/terraform-provider-familio),
   bump it there (`go get github.com/dmalch/go-familio@vX.Y.Z && make check`) to
   confirm the surface still fits its call sites.

See the **Stability** section of the README for what semver does and does not
promise here.

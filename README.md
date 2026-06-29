# go-familio

Go client for the [familio.org](https://familio.org) genealogy API. Extracted from
[terraform-provider-familio](https://github.com/dmalch/terraform-provider-familio)
so the same HTTP layer is usable from CLI tools, migration scripts, and other
projects.

## Disclaimer

This library is an unofficial integration. familio.org publishes no write API;
its endpoints were reverse-engineered from the tree editor. It is not endorsed,
operated, or sponsored by familio.org, and the endpoints may change or break
without notice. Use it only on your own genealogy data, with a session you
established yourself.

## Install

```bash
go get github.com/dmalch/go-familio
```

## Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    familio "github.com/dmalch/go-familio"
)

func main() {
    cookies := os.Getenv("FAMILIO_COOKIES")
    if cookies == "" {
        log.Fatal("set FAMILIO_COOKIES")
    }

    client, err := familio.NewClient(familio.Options{
        Cookies: familio.CookiesFromHeader(cookies),
    })
    if err != nil {
        log.Fatal(err)
    }

    person, err := client.GetPersonBasic(context.Background(), "<person-uuid>")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("name: %s\n", person.DisplayName)
}
```

A runnable version of this example lives in
[`examples/getperson/`](examples/getperson).

## Command-line tool

`cmd/familio` is a read-only CLI façade over the library — handy for quick
lookups (`familio person get`, `familio settlement get`, `familio whoami`, …)
without writing Go:

```bash
go install github.com/dmalch/go-familio/cmd/familio@latest
familio settlement persons <uuid>      # public, no auth
FAMILIO_COOKIES='t=eyJ…' familio whoami
```

See [`cmd/familio/README.md`](cmd/familio/README.md) for the full command list,
auth, and flags.

## Auth

familio's auth is two-layer. The `t` session cookie alone is rejected by the
authed API; familio's Next.js SSR embeds a short-lived JWT bearer in the page's
`__NEXT_DATA__`. The client takes the `t` cookie, scrapes that JWT from an HTML
page, caches it (refreshing ~5 minutes before its `exp`), and sends it as
`Authorization: Bearer` on `/api/v2/*` calls. The JWT's `uuid` claim is the
account id, exposed via `Client.AccountUUID`.

Supply the session cookie via `Options.Cookies`, built with one of:

```go
familio.CookiesFromHeader("t=eyJ…; …")  // raw DevTools / $FAMILIO_COOKIES header
familio.CookieFromSessionToken("eyJ…")   // bare `t` value / $FAMILIO_SESSION
familio.CookiesFromBrowser("chrome")     // a logged-in browser (via sweetcookie)
```

The settlement-persons read is public and needs no credentials.

## Behaviour

- Rate-limited requests (via `golang.org/x/time/rate`); override with
  `Options.RateLimit`.
- Retries on `429` and transient `5xx` responses.
- JWT bearer cached and refreshed automatically with a 5-minute skew before
  expiry.
- `ErrNotLoggedIn`, `ErrNotFound`, and `ErrAccessDenied` map the auth/404/403
  cases for `errors.Is` checks.

## Documentation

API reference: <https://pkg.go.dev/github.com/dmalch/go-familio>

The reverse-engineered HTTP surface (endpoints, request/response shapes, the
auth model) is documented in [`API.md`](API.md).

## Contributing

```bash
make test     # unit tests (no network)
make lint     # golangci-lint
make check    # build + vet + lint + test (CI parity)
```

The live decode test self-skips unless `FAMILIO_NETWORK_TEST=1` is set; run
`make test-acceptance` to exercise it against production data before pushing
changes that touch endpoint or wire-shape code. CI does not run it.

## License

Apache-2.0. See [`LICENSE`](LICENSE).

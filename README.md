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

## What it covers

| Area | Reads | Writes |
|---|---|---|
| Account | `GetProfile`, `AccountUUID` | — |
| Persons | `GetPersonBasic`, `GetPersonRegular`, `GetPersonDisplay`, `GetPersonEvents`, `ListSettlementPersons` and `SearchPersons` (both public) | `CreatePerson`, `UpdatePersonBasic`, `DeletePerson` |
| Tree | `GetTreeGraph` (the editor canvas in one request), `CrawlTree` (bounded BFS, structured dates) | — |
| Events | via `GetPersonEvents` + `DeriveRelations` | `CreateEvent`, `DeleteEvent` (marriages are `wedding` events) |
| Biography | `GetPersonBiography` | `UpdatePersonBiography` |
| Sources | `GetPersonSources` | `CreateSource`, `UpdateSourceComment`, `DeleteSource` |
| Settlements | `GetSettlement` | — |
| History («История изменений») | `ListPersonsHistory`, `GetHistoryFilters` | — (read-only audit log) |
| Matches («Совпадения») | `ListMatches`, `ScrollMatches`, `GetMatchFilters` | `ConfirmMatches`, `RejectMatches`, `UndecideMatches` |
| Tags («Метки») | `ListTags`, `GetPersonTags`, `GetTagsByPersons` | `CreateTag`, `UpdateTag`, `DeleteTag`, `AssignPersonTags`, `UnassignPersonTags` |
| Anything else | `DoRaw`: any `/api/v2` request with the client's auth, rate limit and retry, the response returned undecoded | same |

Deliberately not covered: photo upload, the source **catalog browsing**
endpoints (so `CreateSource` needs a reference uuid you obtained elsewhere), the
`validate/*` helpers, and the matches bulk `*-by-filters` writes. See
[`API.md`](API.md) › Known limitations.

## Command-line tool

`cmd/familio` is a CLI façade over the library — handy for quick lookups
(`familio person get`, `familio tree`, `familio settlement get`, …) and a few
targeted writes (`familio marriage create/delete`, `familio person
set-biography`) without writing Go:

```bash
go install github.com/dmalch/go-familio/cmd/familio@latest
familio settlement persons <uuid>      # public, no auth
FAMILIO_COOKIES='t=eyJ…' familio whoami
familio tree <uuid> -up -surname Иванов
familio api 'users/{owner}/tags'        # any endpoint, like gh api
```

See [`cmd/familio/README.md`](cmd/familio/README.md) for the full command list,
auth, and flags.

## Auth

familio's authed API does not accept a cookie — it wants a JWT in
`Authorization: Bearer`, and there is no endpoint that mints one. The client gets
one of two ways, transparently:

1. **From the cookie.** familio's `t` session cookie value *is* a JWT, so when it
   carries one that is still valid the client uses it directly.
2. **From the page.** Otherwise it fetches a familio.org HTML page with the
   cookie and scrapes the JWT familio's Next.js SSR embeds in `__NEXT_DATA__`.

Either way the token is cached and refreshed ~5 minutes before its `exp`. The
JWT's `uuid` claim is the account id, exposed via `Client.AccountUUID` and used
as `?owner=` on creates.

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
- Every response `>= 400` is an `*APIError` carrying the method, path, status and
  body, wrapping a sentinel where one applies — so `errors.Is(err,
  familio.ErrNotFound)` and a status check both work:

  ```go
  if errors.Is(err, familio.ErrConflict) { /* stale X-Base-Version: re-read */ }

  var apiErr *familio.APIError
  if errors.As(err, &apiErr) { log.Print(apiErr.StatusCode, apiErr.Body) }
  ```

  The sentinels are `ErrNotFound` (404), `ErrNotLoggedIn` (401 or a login
  redirect), `ErrAccessDenied` (403), `ErrInvalidRequest` (400), and
  `ErrConflict` (409 — a stale `X-Base-Version` on `/basic`, `/biography`, or a
  source comment). familio answers some other things with a 409 too. The client
  recognizes them by their body:
  - a **missing person** is `ErrNotFound` (see API.md › Missing persons);
  - a missing parameter or a malformed uuid is `ErrInvalidRequest`;
  - a filter that needs a session is `ErrNotLoggedIn`.
  `Body` has familio's `\uXXXX` escapes decoded, so Cyrillic messages read as text.
- Derived views on top of the raw events: `DeriveRelations(events, uuid)` →
  normalized `parents`/`spouses`/`children` (spouses carry the wedding-event
  "union" uuid), `BirthYear`/`DeathYear`, and `Client.CrawlTree` for a bounded
  BFS over the connected persons.

## Stability

Semantic versioning applies to the **Go API** — the exported identifiers of
package `familio`. Those will not change incompatibly within a major version.

**`cmd/familio` is best-effort.** Its commands, flags and JSON output may change
incompatibly in a minor release; each such change is called out in
[`CHANGELOG.md`](CHANGELOG.md). The CLI is a convenience façade, and tying it to
the same guarantee would mean a flag rename forces a `/v2` module path on library
importers who never touch it. Pin an exact version if you script against it.

`FAMILIO_BASE_URL` (the API host override) *is* stable: it forwards to the
exported `Options.BaseURL`, so it is as supported as that field. Useful for
pointing the CLI at a mock or a proxy.

It cannot apply to familio.org. The endpoints here are reverse-engineered from a
web app that publishes no API and makes no compatibility promise, so:

- **An upstream break is fixed in a patch release**, not a major bump. If
  familio changes a response shape, the fix that follows it is `x.y.Z`.
- New endpoint coverage is a minor release.
- Live decode tests (`make test-acceptance`) are how upstream drift gets caught;
  CI cannot run them, so they run before releases.

The library is only meant for your own genealogy data with a session you
established yourself.

## Documentation

API reference: <https://pkg.go.dev/github.com/dmalch/go-familio>

The reverse-engineered HTTP surface (endpoints, request/response shapes, the
auth model) is documented in [`API.md`](API.md).

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). In short:

```bash
make check    # build + vet + lint + test — the same gates CI runs
```

## License

Apache-2.0. See [`LICENSE`](LICENSE).

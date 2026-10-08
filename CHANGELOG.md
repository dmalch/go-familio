## 1.1.0

### NEW

- **`Client.DoRaw`** sends an arbitrary request to familio.org's `/api/v2` and
  returns the answer undecoded, as a `RawResponse` (status, headers, the whole
  body). It is the escape hatch for endpoints this package does not model. It
  goes through the same rate limiter, retry, User-Agent and JWT bearer as every
  typed call.
  - The endpoint may be API-relative (`profile`), rooted (`/api/v2/profile`) or a
    full URL under `BaseURL`. A URL on any other host is refused before a request
    is made, so the bearer never leaves familio.org.
  - A status `>= 400` is returned, not turned into an error, so a caller can show
    the server's message in full rather than the truncated `APIError.Body`.
  - The bearer is attached only when the client holds a `t` session cookie.
    Without one the call goes out anonymously, which the public settlement list
    accepts, and no token scrape is attempted.

### CLI

- **New `api` command**, in the spirit of `gh api`: `familio api <endpoint>` calls
  any `/api/v2` endpoint with the CLI's credentials and prints the response.
  - It takes gh's flags: `-X`, `-f`/`-F` (typed values, `@file`, `@-`), `-H`,
    `-input`, `-i` and `-paginate`.
  - `{owner}` in the endpoint, or in a `-F` value, becomes the account uuid.
  - Fields go in the query on a GET and form a JSON object otherwise.
  - `-paginate` follows both of familio's pager envelopes, page numbers and the
    `lastItem` cursor, on GETs and on POST reads.
  - JSON responses are re-indented with the API's `\uXXXX` escapes decoded, so
    Cyrillic is readable.
  - A status outside 2xx prints the body and exits 1.

  `cmd/familio/README.md` › "`api` — raw calls" has the details.

## 1.0.2

### FIXED

- **A missing person was reported as a version conflict.**
  - **Cause:** familio answers a person that does not exist with HTTP **409**, not 404, on
    `GET /persons/<uuid>` («Персона не найдена», code 2604) and `GET /persons/<uuid>/sources`
    («Не найдена персона …»).
  - **What went wrong:** `GetPersonRegular`, `GetPersonDisplay`, `GetPersonSources` and
    `UpdateSourceComment` returned `ErrConflict`, which means "re-read and retry", for a
    deleted person, instead of `ErrNotFound`. A Terraform `familio_source` whose person was
    deleted outside Terraform therefore failed to refresh instead of being dropped.
  - **Now:** a 409 whose body reports the resource missing is `ErrNotFound`. Any other 409
    stays `ErrConflict`. The `APIError` keeps the real `StatusCode` (409). `Unwrap` reads
    `Body` as well as `StatusCode`, so a hand-built `APIError` behaves the same.
- **Error messages showed Cyrillic as `\uXXXX` escapes.** familio escapes every non-ASCII
  character, and `APIError.Body` carried that through to every CLI error and Terraform
  diagnostic: `"message":"\u041f\u0435…"`. `Body` now has the escapes decoded, with the key
  order kept. As a side effect, a typical message also fits the 300-byte cut. The cut now
  falls on a rune boundary.

### TESTS & DOCS

- API.md › "Missing persons" records how each person read answers a person that does not
  exist, confirmed live:
  - `/events` answers **200 `[]`**, so `GetPersonEvents` cannot tell "missing" from "no events";
  - `/tags` answers **403**, the same as for another account's person.

  Both are noted on the methods.

## 1.0.1

Documentation only — no code changes.

### CHANGED

- **The stability promise is narrowed to the library.** 1.0.0 said semver covered
  package `familio`'s exported identifiers *and* `cmd/familio`'s commands and flags.
  It now covers the **library only**; the CLI is best-effort, and its commands, flags
  and JSON output may change incompatibly in a minor release, each noted here.

  This reduces what was promised a release earlier, which is worth stating plainly.
  Two reasons:
  - **One module, two audiences.** Honouring the wider promise meant a CLI flag
    rename was a breaking change, and in Go that forces the module path to
    `…/go-familio/v2` — an import-path migration for library consumers who never run
    the CLI.
  - **It protected the wrong thing.** "Commands and flags" said nothing about JSON
    output shape, which is what scripts actually depend on. So it constrained
    development where that was expensive and stayed silent where users were exposed.

  If you script against the CLI, pin an exact version.

- **`FAMILIO_BASE_URL` is documented as stable**, rather than as an internal testing
  knob. It forwards to the exported `Options.BaseURL`, which semver already covers,
  so disclaiming it bought nothing and only set a trap for anyone pointing the CLI at
  a mock or proxy.

## 1.0.0

**No functional changes from 0.7.1** — this release is the commitment, not new
code. The surface worth freezing landed across 0.6.0 and 0.7.x; what changes here
is what you can rely on.

### What 1.0 promises

Semantic versioning applies to the **Go API** — the exported identifiers of package
`familio` — and to `cmd/familio`'s commands and flags. Neither will change
incompatibly within v1.

### What it cannot promise

familio.org publishes no API and makes no compatibility promise; every endpoint
here was reverse-engineered from the web app. So:

- **An upstream break is a patch release**, not a major bump. If familio changes a
  response shape, the fix that follows is `1.y.Z`.
- New endpoint coverage is a minor release. The deliberate gaps — source catalog
  browsing, photo, the `validate/*` helpers, the bulk match writes — are listed in
  `API.md` and can each arrive in a `1.y.0` without breaking anything.
- Live decode tests (`make test-acceptance`) are how upstream drift is caught,
  since CI cannot run them. They run before releases.

See the README's **Stability** section, and `CONTRIBUTING.md` for the release flow.

### Getting here from 0.6.x

One breaking change, in 0.7.0: `CreatePerson` and `GetPersonDisplay` return
`*CreatedPerson` and `*PersonDisplay` instead of unexported types. Code binding
the result with `:=` needs no change. Everything else in 0.7.x was additive or a
fix.

## 0.7.1

### FIXED

- `APIError.Unwrap` now derives its sentinel from `StatusCode` instead of a hidden
  field set at construction. Every field of the struct is exported, so an
  `APIError` a caller builds themselves — simulating a 409 in their own tests, say
  — behaves exactly like one this package produced:
  `errors.Is(&APIError{StatusCode: 409}, ErrConflict)` was **false** in 0.7.0 and
  is now true, and the message names the reason. Errors produced by the client were
  unaffected either way.

## 0.7.0

The API-hygiene release ahead of 1.0: the changes that would need a major bump if
they landed after the freeze. See the README's new **Stability** section for what
semver covers here.

### BREAKING

- `CreatePerson` and `GetPersonDisplay` returned **unexported** types
  (`*createResponse`, `*personDisplay`) — types a caller could not name and godoc
  would not document. They are now `*CreatedPerson` and `*PersonDisplay`, with
  the same fields. Code that binds the result with `:=` needs no change.

### NEW

- **Structured HTTP errors.** Every response `>= 400` now comes back as an
  `*APIError` carrying `Method`, `Path`, `StatusCode` and a truncated `Body`,
  reachable with `errors.As`. It **wraps** the sentinel for its status, so
  existing `errors.Is(err, familio.ErrNotFound)` checks keep working unchanged.
- `ErrConflict` maps **409** — a stale `X-Base-Version` optimistic-lock token on
  `/basic`, `/biography` or a source comment. Previously indistinguishable from a
  400 validation failure without matching on the error string.
- `GetProfile` reads `GET /api/v2/profile`: the account's uuid and email plus the
  account holder's name and gender. `AccountUUID` still answers the uuid alone
  from the JWT claim, with no request.
- `GetTreeGraph` reads `GET /api/v2/tree` — familio's whole tree-editor canvas in
  **one request**, centred on the account's own person. Each node carries its
  layout position (role, Russian kinship label, generational layer, parent and
  partner edges) *and* a person summary: names, locality, photo, pre-formatted
  dates. Three things to know, all documented on the types and in API.md:
  - a **`nodeId` is not a person uuid** — it is `<uuid>.<n>`, since one person can
    be placed twice. Use `TreeGraphNode.PersonUUID()` or `PersonUUIDFromNodeID`,
    because the parent/partner edges speak node ids.
  - the graph is a **window**: `Params.HasMore` marks nodes whose further
    relatives the response omits.
  - the dates are **display strings** («После 29.04.1926», Julian marked «ст.»),
    not parseable values. `CrawlTree` remains the way to get structured dates and
    event uuids.
- Exported `Version`, which feeds the default `User-Agent`. That header
  previously identified the library as `terraform-provider-familio/0.1`.

### FIXED

- **Cookies whose value needs encoding were being mangled.** familio's `t` cookie
  holds a JSON object, and `net/http` silently drops the `"` bytes that are
  illegal in a cookie value — so the credential arrived corrupted and familio
  answered 401. Values that need it are now percent-encoded, as a browser sends
  them (already-encoded values are left alone). This is what made
  `CookiesFromBrowser` look like it returned a stale session.

### CHANGED

- **Auth skips the page fetch when it can.** familio's `t` cookie carries the
  bearer — it is a JSON envelope `{"token":"eyJ…","synapseToken":"syt_…"}` — so
  the client now takes the token from it directly and only falls back to fetching
  and scraping `__NEXT_DATA__` when the cookie is opaque, has no usable token, or
  is near expiry. Same credentials, one less request per refresh; the fallback
  keeps it working if familio changes the cookie. The envelope is parsed as JSON,
  never sniffed: its inner JWT contributes exactly two dots, so a naive parser
  will "successfully" decode the raw envelope and then send the whole thing as the
  bearer.
- `ErrAccessDenied` was documented as covering 401 and 403; 401 has always mapped
  to `ErrNotLoggedIn`. Comment corrected, behaviour unchanged.
- An `*APIError`'s message names what a mapped status means, so a bare 401 still
  reads `not logged in` in a CLI or Terraform diagnostic.

### CLI

- `whoami` now prints the whole account record (uuid, email, display name)
  instead of just the uuid.
- New `graph` command: the tree's node ids and parent/partner edges in one
  request. It is top-level rather than `tree graph` because `tree` is itself a
  command taking a uuid.
- `FAMILIO_BASE_URL` overrides the API host. It exists to point the CLI at a test
  server; it is env-only and not a flag.

### TESTS & DOCS

- The event, source, person-basic and settlement **write paths had no unit
  tests** — the calls with the `ld+json` and `X-Base-Version` subtleties. They do
  now, along with the CLI write commands, the login-redirect guard, the retry and
  body-replay path, and godoc examples. Library coverage 73.9% → 83.0%, CLI
  53.0% → 75.4%.
- Fixed a latent bug in the live tags test: `BeEmpty()` on an `int` id always
  failed, so `TestListTagsLive` could not pass for an account that owns tags.
- New `CONTRIBUTING.md` (gates, fixture policy, live-test and release flow), a
  **Stability** section and a capability table in the README, and a rewritten
  package doc.

## 0.6.0

### NEW

- **Tags («Метки»).** New support for familio's person labels — the `/profile/my-tags` feature.
  `Client.ListTags(ctx)` reads the account's tag catalogue, `CreateTag`/`UpdateTag`/`DeleteTag`
  manage it, and `GetPersonTags`/`AssignPersonTags`/`UnassignPersonTags` manage the links between
  a person and its tags. `GetTagsByPersons(ctx, uuids)` is the bulk read the tree and person-list
  views use, returning a `PersonTags` map keyed by person uuid. See `API.md` › Tags sub-resource.
- `TagInput.Validate` mirrors the rules the web editor applies before it submits: a non-blank name
  within `TagNameMaxLen` (1000), one of the seven palette codes, and a description within
  `TagDescriptionMaxLen` (5000). `CreateTag` and `UpdateTag` call it, so bad input fails without
  spending a request. The exported `TagColors` / `TagColorHex` carry the palette in UI order —
  familio stores a colour *code* (`mint-mist`), never a hex.
- Three shape notes worth knowing when consuming `Tag`. **`ID` is an `int`** — tags are the one
  familio resource keyed by a small sequential integer rather than a uuid, so the assign/unassign
  and update/delete calls take `int`, and `RegularRecord.Tags` (the `regularPerson` view's `tags`)
  is a `[]int` of ids, not tag objects. `AssignPersonTags` returns the person's refreshed
  `[]Tag` because the endpoint echoes it, while `UnassignPersonTags` answers 204 and returns only
  an error. And `IsFree` is server-computed — tags are Plus-gated, and a non-Plus account can
  actually use only one tag, the one flagged free. The API returns the others anyway; nothing here
  enforces the limit.

### CLI

- New `tags list`, `tags person <person-uuid>` and `tags by-persons <person-uuid>…` reads, plus
  `tags colors`, which prints the accepted palette codes with their hexes and makes no request.
- New `tags create` / `tags update <tag-id>` writes taking `-name`, `-color` and `-description`.
  Invalid input is reported before any request is made.
- New `tags delete <tag-id>…` and `tags assign` / `tags unassign <person-uuid> <tag-id>…`. Like
  the `matches` mutations they prompt `[y/N]` on stderr, listing what they are about to touch;
  `-yes` skips the prompt for scripted use, and declining or an empty stdin aborts without a
  request. `assign`/`unassign` print the person's refreshed tag list afterwards.

## 0.5.0

### NEW

- **Matches («Совпадения»).** New support for familio's duplicate-candidate inbox — the monthly
  pass that pairs your persons with other users' public persons and with record-catalog entries.
  `Client.ListMatches(ctx, MatchFilter)` pages through
  `POST /api/v2/users/<accountUuid>/matches/get-by-filters` with all the UI's filters (persons,
  foreign owners, catalogs, batch dates, statuses and the score window),
  `Client.ScrollMatches(ctx, filter, after)` walks the same set through the cursor endpoint, and
  `Client.GetMatchFilters(ctx, filter)` fetches the facet vocabularies with counts. A `Match`
  exposes `{OwnPerson, ForeignPerson}` as `MatchPerson` — the existing `Person` read shape plus
  the ownership fields — so a foreign side is either a `regularPerson` from another tree
  (with `OwnerID`) or a `catalogPerson` from a record catalog (with `CatalogKey`/`CatalogName`).
  See `API.md` › Matches sub-resource.
- **Match decisions.** `Client.ConfirmMatches`, `RejectMatches` and `UndecideMatches` set the
  status of matches by match uuid via the `*-by-ids` endpoints. All three are reversible:
  `UndecideMatches` returns a match to `undecided`. The filter-wide `*-by-filters` bulk endpoints
  are documented in `API.md` but intentionally not implemented — their request body disagrees with
  the read endpoints on how `status` is encoded, and verifying that would mean mass-mutating real
  matches.
- `HistoryFacet` is now an alias for the new shared `Facet` type, which the matches facets reuse.
  The shape is unchanged, so existing code keeps compiling.

### CLI

- New `matches list` command with `-status`, `-person`, `-user`, `-catalog`, `-date`,
  `-min-score`/`-max-score` and `-page`/`-limit` flags, plus `-all` to sweep every page through
  the scroll cursor, and `matches filters` for the facet counts (the same filter flags narrow
  them).
- New `matches confirm`, `matches reject` and `matches undecide` commands taking one or more match
  uuids. They list the affected matches and prompt `[y/N]` on stderr before acting; `-yes` skips
  the prompt for scripted use. Declining, or an empty stdin, aborts without issuing a request.

### FIXED

- **`FlexDate` now marshals back to a plain date string.** It had an `UnmarshalJSON` but no
  `MarshalJSON`, so re-encoding a `Person` leaked the struct's untagged fields as
  `"birthDate": {"Formatted": "1890", "Present": true}`. It now encodes as `"1890"` (or `null`
  when there is no date), mirroring the string form it already accepts on read. This changes the
  rendered output of `settlement persons` and of both person sides of `matches list`.

## 0.4.0

### NEW

- **Person change history («История изменений», Familio Plus).** New read-only support for the
  audit log familio shipped on 2026-07-08: `Client.ListPersonsHistory(ctx, HistoryFilter)` pages
  through `GET /api/v2/persons/history/<accountUuid>` with all the UI's filters (text, operations,
  causes, authors, persons, data types, date range) and `Client.GetHistoryFilters(ctx)` fetches the
  facet vocabularies with counts. Entries expose `{Record, Person, Author}`; `Record.Changes` is
  the block-shaped snapshot kept as raw JSON (the API carries no before/after diff — the UI
  computes it client-side). See `API.md` › Change history sub-resource.

### CLI

- New `history list` command with `-person`, `-author`, `-operation`, `-cause`, `-block`
  (`-event-type`/`-source-type`), `-text`, `-from`/`-till`, `-page`/`-limit`, and `-asc` flags,
  and `history filters` for the facet counts.

## 0.3.1

### FIXED

- **Source comment edit now sends the optimistic-lock header.** familio guards the source
  comment `PATCH` with the same `X-Base-Version` header as `/basic` and `/biography` (its value
  is the source's own `updatedAt`); without it the edit is rejected with «Не указана дата-время
  последнего обновления источника» (HTTP 400/409). `Client.UpdateSourceComment` now reads the
  source's current `updatedAt` and sends it, so creating/editing a source with a comment works
  again. Signature unchanged. `API.md`'s sources section corrected (it wrongly said no
  `X-Base-Version` is involved).

## 0.3.0

### NEW

- **Normalized relations & derived person view.** New `DeriveRelations(events, uuid)` reduces a
  person's events into `Relations{Parents, Spouses, Children}` — flat `PersonRef` lists instead of
  per-event participant roles. Spouses are `Spouse{UUID, Name, MarriageUUID}`, exposing the
  underlying wedding-event (union) uuid needed to import a `familio_marriage` or target it for
  deletion. `BirthYear`/`DeathYear` and `OwnDeathEvent` helpers complete the reduction. (#4, #5)
- **Tree crawler.** `Client.CrawlTree(ctx, rootUUID, TreeOptions{Direction, Surname, Depth})`
  breadth-first walks the connected persons around a root and returns `[]TreeNode`
  (`{uuid, name, year, parents, spouses, children}`), replacing hand-written BFS crawlers.
  Direction is `TreeUp`/`TreeDown`/`TreeComponent`; `Surname` bounds expansion to keep in-law
  branches out; `Depth` caps distance. (#3)

### CLI

- `person get` now also emits `relations`, `birthYear`/`deathYear`, `birthDate`/`deathDate`, and a
  `marriageUuid` on each spouse — alongside the raw `basic` and `events`. (#4, #5)
- New `tree <uuid> [-up|-down|-component] [-surname <s>] [-depth <n>]` command. (#3)
- New write commands: `marriage create <a> <b> [-date] [-comment]`,
  `marriage delete <person-uuid> <union-uuid>`, and
  `person set-biography <uuid> [-text|stdin] [-append]`. (#6)
- **Global flags may now appear after the subcommand and its arguments**
  (`person get <uuid> -browser chrome`), not just before it. (#8)
- All machine-readable output emits **full uuids** consistently — no truncated prefixes. (#7)

## 0.2.0

### NEW

- **Person biography support.** New `Biography{Text, UpdatedAt}` value object plus
  `Client.GetPersonBiography` (GET `/persons/<uuid>/biography`) and
  `Client.UpdatePersonBiography(uuid, text, version)` (PUT with `X-Base-Version`). The
  biography sub-resource carries its **own** optimistic-lock version, distinct from `/basic`'s.
  `CreatePersonInput` gains an optional `Biography *string` to set the initial value at create
  time. See `API.md` › Biography sub-resource.

## 0.1.0

### NEW

- Initial release. The familio.org HTTP client, extracted verbatim from
  [terraform-provider-familio](https://github.com/dmalch/terraform-provider-familio)'s
  `internal/familio` package so the same HTTP layer is usable from CLI tools,
  migration scripts, and other projects.
  - `familio` package: `Client` with person CRUD, life-fact events, sources,
    wedding (marriage) events, the public settlement-persons list, and
    settlement lookup. Two-layer auth (session `t` cookie bootstraps a scraped
    JWT bearer). Cookie helpers: `CookiesFromHeader`, `CookieFromSessionToken`,
    `CookiesFromBrowser`. Date translation between the domain `DateRange` and
    familio's wire `EventDate`.
  - `AccountUUID(ctx)` accessor exposing the authenticated account uuid from
    the JWT `uuid` claim.
  - `cmd/familio`: a read-only CLI — `whoami`, `person get`, `settlement get`,
    `settlement persons`, `sources list`.

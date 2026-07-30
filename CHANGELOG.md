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

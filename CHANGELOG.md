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

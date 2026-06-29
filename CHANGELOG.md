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

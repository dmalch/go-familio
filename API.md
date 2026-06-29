# Familio.org API notes

This file is the source of truth for familio.org's HTTP surface as used by the provider.
familio publishes no official write API and no OpenAPI/Hydra docs; everything here was
reverse-engineered by driving the logged-in web editor and capturing + replaying the network
calls (see [Reverse-engineering method](#reverse-engineering-method) at the end). Both the
read and write paths are confirmed and implemented.

## Backend & conventions

- Backend is **API-Platform / Hydra**. Send `Accept: application/ld+json` and, on writes,
  `Content-Type: application/ld+json`; list responses use `hydra:member` / a `pager` envelope.
- `familio.org/api/v2/*` is a Next.js proxy in front of the CORS-locked `coral.familio.org`
  backend (host discoverable via `GET /api/v2/coral/path`). The backend is reachable directly
  server-side with the bearer — the CORS lock is browser-only.
- No public API docs (all `…/docs*` 404).
- Session cookies seen: `t` (session, HttpOnly), DataDome anti-bot (`__ddg*`), and
  `cookieConfirmed` / `records_spoiler` (non-auth).

## Authentication — two-layer (cookie bootstraps a JWT bearer)

The `t` session cookie is **not** accepted by the authed API directly
(`GET /api/v2/profile` with only the cookie → **401** «Требуется авторизация»). The real
credential is a **JWT bearer**:

1. Logging in in a browser sets the **`t`** session cookie.
2. familio's Next.js SSR reads `t` and embeds a **JWT** in the page's `__NEXT_DATA__`
   (`…initialState… "token":"eyJ…"`). It is RS256; payload
   `{ iat, exp, roles:["ROLE_USER"], uuid:"<userId>" }`; expiry ≈ **30 days**.
3. Every `/api/v2/*` call carries `Authorization: Bearer <jwt>`. With the bearer,
   `GET /api/v2/profile` / `/tree` / `/persons/<uuid>` return **200**; without it, **401**.

There is no token-mint endpoint (`/api/v2/auth/*`, `/me`, `/token` all 404) — the
SSR-embedded token is the only source.

**Provider implication:** from the `t` cookie the client fetches a familio.org HTML page,
scrapes `__NEXT_DATA__.token`, and sends it as `Authorization: Bearer`. The JWT `uuid` claim is
the account id, used as `?owner=<userId>` on creates. The JWT is re-scraped near expiry. The
only public, **no-auth** endpoint is `GET /api/v2/persons?settlement=` (the settlement list).

## Core model — relationships & life facts are EVENTS

familio has **no `union`/relationship resource**. Kinship and life facts are all **events**
carrying a `participants[]` list, attached under a person:

- **Marriage/partnership** = a `wedding` event with two `spouse` participants.
- **Parent↔child** = a `birth` event whose participants are the one `child` plus 0–2
  **gender-agnostic `parent`s**. `father`/`mother` are **not** valid roles (rejected «Не
  определена роль участника»); a parent's father/mother display is inferred from that person's
  own gender.
- **Single-subject facts** (death, baptism, residence, …) use a single `owner` participant.
- **`godparent` (Восприемник) / `warranter` (Поручитель)** are two-person events where **both**
  participants are `owner` (symmetric — direction is not stored, like marriage's two `spouse`s).

**Participant roles** (full vocabulary): `child`, `parent`, `sibling`, `spouse`, `owner`. Each
event type restricts which roles are valid (e.g. `child`/`parent` only on `birth`, `spouse` on
`wedding`/`divorce`/`affiance`/`nikah`).

`"personUuid":"self"` is the placeholder for the person being created in the **same**
`POST /api/v2/persons` request (resolved server-side to the new uuid).

**Two event classes** (matters for editing):

- **Unique / keyed** — `birth` (keyed by its `child` participant) and `death` (one per person).
  Re-POSTing **upserts** (full replace in place). `birth` is mandatory; deleting the sole birth
  event is **409**, so birth is upsert-only.
- **Repeatable facts** — `baptism` and the rest. Re-POSTing **duplicates** (does not upsert), so
  editing in place means **DELETE the old event + POST a new one**.

### Dates

A date is `{ calendar:"gregorian"|"julian", type:"equal"|"about"|"before"|"after"|"between"|…,
first:{day,month,year,type}|null, second:{…}|null }`. `formatted` is server-computed
(`"Неизвестно"` when empty); never sent. `type` is the whole-date qualifier (approximation /
bound / range); each part's own `type` is its calendar. Validate via
`PUT /api/v2/validate/complex-date` (204 = ok); surnames via `POST /api/v2/surnames/validate`
`{surname}` (204). See `internal/familio/date.go` for the domain ⇄ wire translation.

### Settlement / place on events

Any event (`birth`, `death`, `baptism`, `location`, …) carries an optional **`settlement`** —
familio's «Место рождения / смерти». It is a **structured object, not a bare uuid**:

- **Write (minimal accepted):** `"settlement": {"uuid":"<settlement-uuid>"}` → **201**; the
  server enriches the rest. A bare string `"settlement":"<uuid>"` → **400** «Ошибка(и) в данных
  запроса». `null` ⇒ no place / clears it.
- **Read-back:** `"settlement": {"uuid","name","mainGeorequisite":{level1,level2,year}}`. (The
  `regularPerson` view's `birthPlace`/`deathPlace` is a richer `{uuid, primaryName,
  additionalNames, mainGeorequisite, type, status, coordinate}` — same uuid.)
- Settlement rides the **same birth/death POST-upsert** as the date, so it must be re-sent on
  every upsert (a full replace would otherwise clear it).
- **Resolve / validate a uuid:** `GET /api/v2/settlements/<uuid>` (**Bearer**) → **200**
  `{uuid, primaryName, additionalNames[], mainGeorequisite:{level1,level2,year}, type, status,
  coordinate, nearestSettlements[]}`. `coordinate` is **GeoJSON** `{type:"Point",
  coordinates:[lon,lat]}`. `type`/`status` are Russian labels (e.g. «село», «жилой»). Not needed for
  writes (server enriches `{uuid}`); consumed by the `familio_settlement` data source. Only the
  plural path works — `/settlement/<uuid>` and `/geo/settlements/<uuid>` → 404.

## Endpoint reference

### Persons — public read

`GET /api/v2/persons?settlement=<uuid>&itemsPerPage=<n>&page=<n>` — **no auth**. All persons
(catalog-sourced + user-created) linked to a settlement:

```jsonc
{ "pager": { "page": 1, "itemsPerPage": 300, "totalItems": 19885 },
  "data": [ {
    "uuid": "85781e3b-…", "displayName": "Августа Степановна", "shortDisplayName": "…",
    "catalogKey": "mkzhuravkinotambov", "catalogName": "Метрические книги …",
    "type": "catalogPerson",
    "birthDate": null, "deathDate": null, "hasDeathEvent": false,
    "birthSettlementText": "", "updatedAt": "2025-02-28T…" } ] }
```

- `catalogKey` is `null` for **user-created** profiles (the provider's write target).
- No server-side catalog facet (`catalog=` + `settlement=` ⇒ `totalItems: 0`); filter
  `catalogKey` client-side.
- Keep `itemsPerPage` ≤ 300 (backend timeouts); page until a short/empty page.

### Persons — authed read (Bearer)

- `GET /api/v2/profile` → `{ user:{uuid,email,…}, profile:{displayName,firstName,lastName,
  middleName,gender,…} }` — the current account.
- `GET /api/v2/tree` → `{ nodes:[{ nodeId, nodeParams:{ role, parents:[{sex,nodeId}],
  partners:[…] } }] }` — the tree graph.
- `GET /api/v2/persons/<uuid>` → the `regularPerson` view:
  `{ uuid, type:"regularPerson", displayName, originalDisplayName, shortDisplayName, ownerId,
  gender, birthPlace, deathPlace, deathSettlementText, photo, biography, isMine, isMe,
  canBuildTree, privacyType, updatedAt, tags, isGrantedToMe }`. `ownerId` (the owning account)
  is here but **not** on the public settlement list.
- `GET /api/v2/persons/<uuid>/basic` → `{ uuid, createdAt, updatedAt, gender, privacy,
  firstName, lastName, middleName, birthLastName, birthFirstName }` — the edit-form source.
- `GET /api/v2/persons/<uuid>/events` → `[{ uuid, type, date, settlement, comment,
  participants, … }]`.
- Frontend routes (`_buildManifest`): `/persons/new`, `/persons/new/simple/[id]`,
  `/persons/[personId]`, `/my-tree`, `/tree`, `/persons`.

### Persons — write (Bearer + `application/ld+json`)

**Create** `POST /api/v2/persons?owner=<userId>` → **201**. Only the `birth` event is required:

```jsonc
{
  "basic": {
    "firstName": "Иван", "lastName": "Иванов", "middleName": "Иванович",
    "birthFirstName": "", "birthLastName": "",     // maiden name at birth
    "gender": "male",                               // male | female
    "privacy": "visible_for_all"                    // | "invisible"
  },
  "photo": null,
  "events": [
    { "uuid": null, "type": "birth",
      "date": { "calendar":"gregorian", "type":"equal", "first":null, "second":null },
      "participants": [ { "personUuid":"self", "role":"child" } ],
      "settlement": null, "comment": "" }
    // optional: { "type":"death", participants:[{personUuid:"self",role:"owner"}], … }
  ],
  "biography": null
}
```

Response `201` → `{ basic:{ uuid, displayName, firstName, …, createdAt, updatedAt }, photo,
events:[{ uuid, type, date:{…,formatted}, participants:[{personUuid,role,displayName,gender}],
settlement, comment, … }] }`. **New person uuid = `basic.uuid`.**

**Update basic** `PUT /api/v2/persons/<uuid>/basic` → **200**. Body is just the basic fields:

```jsonc
{ "firstName":"…", "lastName":"…", "middleName":"…", "birthFirstName":"", "birthLastName":"",
  "gender":"female", "privacy":"invisible" }
```

The optimistic-lock token is the **`X-Base-Version` HTTP header** (not a body field); its value
is the `updatedAt` last read from `GET /basic`. Missing → **400** «Не указана дата последнего
обновления информации»; stale → **409 Conflict**. The response echoes `/basic` with a bumped
`updatedAt`. The same header guards `/basic`, `/biography`, and `/source`. (Editing the photo
also fires `DELETE /api/v2/persons/<uuid>/photo`.)

**Biography sub-resource** `…/persons/<uuid>/biography` (Bearer). A person's free-text life
description (the web "tab=2" panel) is its **own** sub-resource with its **own** version,
distinct from `/basic`'s:

- **Read** `GET …/biography` → **200** `{ "text": "<biography>", "updatedAt": "<RFC3339>" }`.
  Empty biography is `text: ""` (not null). The text lives in `text` — note the regular
  person view (`GET /persons/<uuid>`) exposes a flat `biography` field, but the editable
  sub-resource uses `{text, updatedAt}`.
- **Write** `PUT …/biography`, body `{ "text": "…" }`, with `X-Base-Version: <the biography's
  own updatedAt from GET /biography>` → **200** `{ "text": "…", "updatedAt": "<bumped>" }`.
  Use the biography's `updatedAt`, **not** `/basic`'s.
- **Create-time:** the `POST /persons` envelope also carries a flat `biography` string for
  setting the initial value.

**Delete** `DELETE /api/v2/persons/<uuid>` → **204**.

### Events sub-resource (Bearer)

- **Create** `POST /api/v2/persons/<personUuid>/events` with one Event body
  (`{uuid:null, type, date, participants:[…], settlement, comment}`) → **201**, returns the
  event with its new `uuid`. The event appears on **every participant's** `/events`, so
  read/delete can anchor on any participant. (`POST /api/v2/events` with no person prefix →
  **404** — events are strictly a person sub-resource.)
- **Delete** `DELETE /api/v2/persons/<personUuid>/events/<eventUuid>` → **204**.
- **In-place edit via POST-upsert** — `PUT …/events/<id>` is blocked by an unknown
  concurrency-token field, but is not needed: re-POSTing a `birth`/`death` event **upserts**
  that person's single event of that type (a **full replace** of participants + date + place,
  not an append):
  - *Birth date / parents / birth place:* POST a `birth` event with `[{child,role:child},
    {parent…}]` + date + settlement. Whatever you send is the new state (omit a parent ⇒
    removed; omit the date ⇒ cleared). The birth event count stays 1.
  - *Death date / place:* POST a `death` event `[{person,role:owner}]` + date + settlement.
  - *Remove death:* `DELETE …/events/<deathUuid>` → 204 (death is optional). Birth is
    upsert-only (deleting the sole birth event → **409**).
- **Related-person shortcut** — the tree UI's "+ Муж/Жена/Отец/Мать/Сын/Дочь" route to
  `POST /api/v2/persons/new/simple/<existingUuid>?role=spouse|parent|child&gender=…`, which
  submits a normal `POST /api/v2/persons?owner=<userId>` whose `events[]` carries the linking
  event (a `wedding` with the existing person as the other `spouse`, or a `birth` with them as
  parent/child). So **creating a related person + its link is one atomic person-create.**

### Event-type catalogue (~50 types)

From the editor's `_app` chunk (key → Russian label): `birth` Рождение, `death` Смерть,
`baptism` **Крещение**, `burial` Похороны, `wedding` Бракосочетание, `divorce` Развод,
`affiance` оглашение, `nikah` Никах, `confirmation` Конфирмация, `naming` Имянаречение,
`location` Место жительства, `education` Образование, `profession`/`occupation` работа,
`militaryService` Военная служба, `militaryAward` Военная награда, `conscription` Призыв,
`captured` Плен, `missing` Пропал без вести, `godparent` Восприемник, `warranter` Поручитель,
`award`, `arrest`, `crime`, `condemnation`, `citizenship`, `immigration`/`emigration`, `hajj`,
`circumcision`, … — the provider exposes types as needed. See the two [event
classes](#core-model--relationships--life-facts-are-events) above for upsert vs. duplicate
behaviour.

### Sources sub-resource (Bearer)

A person's **«Источники»** (sources / record citations — the `?tab=3` panel) are a **separate
sub-resource collection**, *not* events. A source is an immutable **reference** to a catalogued
entity plus a mutable free-text comment. No `X-Base-Version` header is involved (unlike `/basic`).

- **List** `GET /api/v2/persons/<personUuid>/sources` → **200**, a JSON array of source objects.
- **Create** `POST /api/v2/persons/<personUuid>/sources` with the **write body**
  `{ "uuid": "<entityUuid>", "type": "<case|catalog_person>", "catalogKey": <null|string> }`
  → **200**, returns the full (enriched) source object. The write carries only those three
  fields; `name`/`requisites`/`years`/`comment` are server-derived/defaulted.
- **Edit (comment only)** `PATCH /api/v2/persons/<personUuid>/sources/<entityUuid>` with
  `Content-Type: application/ld+json`, body `{ "comment": "…" }` → **200**, returns the updated
  object. Only `comment` is mutable; the reference (`uuid`/`type`/`catalogKey`) is fixed —
  changing it is a different source (delete + create).
- **Delete** `DELETE /api/v2/persons/<personUuid>/sources/<entityUuid>` → **204**.

The path id is the **referenced entity's uuid** (the source's identity within the person), so a
person cannot cite the same entity twice. **Source object (read shape):**
```jsonc
{ "uuid": "58e68fa4-…",        // the referenced entity uuid (= path id); the source's identity
  "type": "case",               // "case" = archive дело; "catalog_person" = a people-catalog record
  "comment": "",                // user free text (PATCH-editable)
  "name": "Ревизские сказки",   // server-derived label (read-only)
  "requisites": "ГИА … ф. 145 оп. 1 д. 431",  // server-derived archive coordinates (read-only)
  "years": "1811 - 1811",       // server-derived (read-only)
  "catalog": null,              // server-derived (read-only)
  "createdAt": "…", "updatedAt": "…" }
```
The two confirmed `type`s come from the two "add source" UI flows:
- **`case`** — «Добавить архивный документ»: a digitised archive *case* (дело) chosen by drilling
  the **organization → fund (Фонд) → register/опись → case (Дело)** catalog
  (`/api/v2/{organizations,funds,registers,cases}`). `catalogKey` is **null**.
- **`catalog_person`** — «Добавить запись из справочника»: a record from a people index, found via
  `GET /api/v2/persons?type=catalogPerson&names=…&bindAllowed=true`. Here `catalogKey` names the
  source catalog (e.g. `"gwarmil"` = the WWI «Памяти героев Великой войны» project), since a
  catalog-person uuid is only unique within its catalog.

## Provider mapping

How the resources use the surface above:

- **`familio_person`** — the `basic` fields, plus the `birth`/`death`/`christening` events as
  nested **blocks**, each grouping its `date`, `place` (a bare settlement uuid wrapped as
  `{uuid}` on write, read back from `settlement.uuid`) and free-text `comment`. The **`birth`**
  block also carries **`parents`** (0–2 uuids — the `parent` participants on the birth event).
  The whole birth block (date/parents/place/comment) and the death block edit **in place** via
  the POST-upsert; christening (a repeatable `baptism`) edits via delete-then-create. Read picks
  the birth event where the person is the `child` (a parent's `/events` also lists their
  children's births). A place/comment is recorded even with an unknown date.
- **`familio_marriage`** — an association resource over a partner pair; POSTs a `wedding` event
  between two existing persons, with an optional `comment`. The **date and comment edit in place**
  (wedding events don't upsert, so — like the christening — the edit deletes the old event and
  creates a fresh one, giving it a new uuid); changing **partners** forces replacement, since the
  pair is the marriage's identity.
- **`familio_event`** — the long tail of single-subject `owner` fact events (location,
  profession, education, military, awards, `godparent`/`warranter`, …).
- **`familio_source`** — a person's source citation (the sources sub-resource above): the
  reference (`reference_uuid` + `type` + optional `catalog_key`) is fixed (RequiresReplace), while
  `comment` edits **in place** via PATCH; `name`/`requisites`/`years`/`catalog` are computed. The
  same source set is also exposed as an authoritative **`sources` block on `familio_person`** —
  the two surfaces are **mutually exclusive per person** (manage a person's sources via the inline
  block *or* via standalone `familio_source` resources, never both; an omitted block leaves a
  person's sources unmanaged).
- **`familio_settlement_persons`** (data source) — the public settlement list above.
- **`familio_settlement`** (data source) — looks up one settlement by uuid via
  `GET /api/v2/settlements/<uuid>`, surfacing its name, requisites (region/district/year), type,
  status and lat/lon (from the GeoJSON coordinate). Resolves/validates the settlement UUIDs the
  place attributes and `familio_source` speak.
- **`familio_person`** (data source) — reads `GET /persons/<uuid>` per uuid for `ownerId` (to
  tell one's own tree from other owners'/catalog rows) and derives relationships from
  `/events`: parents from the own birth event (`OwnBirthEvent`), spouses from wedding events
  (`SpousesOf`), children as the inverse — births where the person is a `parent` (`ChildrenOf`).

## Known limitations & open questions

1. **Wedding events don't upsert** — re-POSTing a `wedding` duplicates it (the upsert trick is
   confirmed only for single-subject `birth`/`death`). So `familio_marriage` edits its date/comment
   in place by **delete-old + create-new** (the new event gets a new uuid), exactly like the
   christening; `partners` stays RequiresReplace by design (a different pair is a different marriage).
2. **`PUT …/events/<id>`** — blocked by an unknown concurrency-token field name; not needed
   while the POST-upsert covers births/deaths and delete+create covers weddings.
3. **Token refresh** — the JWT lasts ~30 days; the client re-scrapes `__NEXT_DATA__.token` near
   expiry rather than via a mint endpoint (none exists).

## Reverse-engineering method

Drive the user's real logged-in Chrome with `playwright-cli attach --extension=chrome` (reusing
the `t` cookie; needs `PLAYWRIGHT_MCP_EXTENSION_TOKEN`), then capture the Network panel while
performing an action in the editor — or replay calls directly from the page context with the
scraped bearer. Record, per action: method · full URL · auth/headers · JSON request body · JSON
response (esp. any new uuid). See the `playwright-cli` skill and the
`playwright_cli_drive_real_chrome` / `familio_authenticated_access` memories.

This performs **real mutations** on the user's account — run only with explicit go-ahead, and
prefer a disposable test person that is deleted afterward (the settlement write contract above
was confirmed exactly this way: create → POST settlement variants → read back → delete).

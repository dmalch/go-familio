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
- Responses escape every non-ASCII character as `\uXXXX` (PHP's `json_encode` default), so
  Cyrillic arrives as `"\u041f\u0435…"`. The fixtures in this repo were unescaped when they
  were trimmed. `APIError.Body` decodes the escapes, so error messages read as text.
  Requests may send raw UTF-8.
- Session cookies seen: `t` (session, HttpOnly), DataDome anti-bot (`__ddg*`), and
  `cookieConfirmed` / `records_spoiler` (non-auth).
- **409 is not only the optimistic lock.** A stale `X-Base-Version` on a write is a 409, but
  familio also answers 409 for:
  - a missing person on some reads (see [Missing persons](#missing-persons));
  - a malformed uuid on `/events`, `/biography` and `/sources`;
  - a missing required query parameter, such as a history `date[from]` without `date[till]`;
  - a person search for the account's own persons (`types[]=my_persons`) without a session.

  The body's `message` says which. The client maps a missing-resource 409 to `ErrNotFound`, and
  every other 409 to `ErrConflict`.

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

There is no token-mint endpoint (`/api/v2/auth/*`, `/me`, `/token` all 404).

**The cookie carries the token.** The distinction is where it travels, not what it is: sending `t` as
a **Cookie** header is rejected (401), but the JWT inside it, sent as `Authorization: Bearer`, is
accepted (confirmed live 2026-07-30). `t` is **not** a bare JWT — it is a URL-encoded JSON envelope:

```jsonc
{ "token": "eyJ…",                    // the /api/v2 bearer
  "synapseToken": "syt_…" }           // familio's Matrix/chat token — NOT a credential here
```

Two traps when reading `t`:

- **Parse it as JSON, don't sniff it.** The inner JWT contributes exactly two dots, so splitting the
  raw envelope on `.` yields three parts and a naive JWT parser will decode the middle one happily —
  then send the whole envelope as the bearer, earning `401 {"code":401,"message":"Invalid JWT
  Token"}`.
- **The value needs percent-encoding on the wire.** It contains `"`, which is illegal in a cookie
  value; Go's `net/http` silently drops such bytes and mangles the credential. Browsers send it
  encoded — match that. (This is why cookies lifted straight out of a browser store used to look
  like a "stale session".)

So the SSR scrape is a fallback, not the only source: it matters when the cookie holds an opaque
value, when the envelope has no usable token, or when the JWT is near expiry and the page will render
a fresher one.

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

### Persons — search (v3, public)

familio's people search («Люди», the `/persons` page) is **`GET /api/v3/persons`**, the one
endpoint this client uses outside `/api/v2`. It searches across the account's own persons,
other accounts' visible persons and the record catalogs. It is **public**: anonymous calls work.
A bearer adds the account's own private persons, and is required for `types[]=my_persons`.
Found through the route table in familio's public JS bundle (`/api/v3` base, `persons.root`),
and confirmed live 2026-10-08 with the counts below for «Мальчиков».

**Required** — each missing one is a **400** naming it: `page`, `itemsPerPage`, `orderBy`,
`orderDirection`, `mentions`. The web UI sends `orderBy=score`, `orderDirection=desc`,
`mentions=false`. `itemsPerPage` up to at least 500 is accepted. A search with **no name
criterion returns 0**.

| param | example | meaning |
|---|---|---|
| `lastName` | `lastName=Мальчиков` | fuzzy (also finds «Мальчеков», «Пальчиков») — 1253 |
| `lastNameExactMatch` | `true` | exact last name — 515 |
| `firstAndMiddleName` (+ `…ExactMatch`) | `Иван` | first and middle name — 97 with the surname |
| `name` | `name=Мальчиков Иван` | free text over the whole name — 186 |
| `types[N]` | `types[0]=catalog_persons` | `my_persons` / `catalog_persons` / `other_users_persons`; none = all — 205 / — / 1048 |
| `gender` | `male` | `male` / `female` — 695 / 353 |
| `birthDate[…]`, `deathDate[…]` | see below | birth / death date filter — 86 for born 1850..1860 |
| `orderBy` | `min_birth_date` | `score`, `full_name`, `birth_settlement_name`, `person_updated_at`, `min_birth_date`, `min_death_date` |

**Dates** take `[calendar]` (`gregorian` / `julian`) and either `[equal][year|month|day]` for one
date (year-only works: 18 born in 1892) or `[from][…]` + `[till][…]` for a range. A range bound
needs **every part**: a missing one is a 400 «Поле [from][day] фильтра birthDate должно быть
числом». The UI widens a year to 1 January .. 31 December and uses years 1 and 9999 for an open
side. `SearchPersons` does the same, ending a month range on the month's real last day.

**Errors:** `types[]=my_persons` without a bearer is a **409** «Фильтр "мои персоны" недоступен
без авторизации». `SearchPersons` returns `ErrNotLoggedIn` before sending it.

**Response** `{ pager:{page,itemsPerPage,totalItems}, data:[…] }`, names without markup:

```jsonc
// regularPerson — an account's tree person
{ "uuid", "type":"regularPerson", "displayName", "shortDisplayName", "originalDisplayName",
  "ownerId", "gender", "privacyType", "isMine", "isMe", "canBuildTree", "isGrantedToMe",
  "tags":[int], "photo", "biography", "updatedAt", "settlementEvents":[],
  "birthPlace":{uuid,primaryName,additionalNames,mainGeorequisite,type,status,coordinate} | null,
  "deathPlace": …, "birthSettlementText", "deathSettlementText",
  "birthDate":{type,calendar,first:{year,month,day,formatted,type},second,formatted} | null,
  "deathDate": …, "hasDeathEvent" }
// catalogPerson — a record-catalog («справочник») entry
{ "uuid", "type":"catalogPerson", "displayName", "shortDisplayName", "originalDisplayName",
  "catalogKey", "catalogName", "updatedAt", "updating", "mentions":[],
  "birthSettlementText", "birthDate", "deathDate", "hasDeathEvent" }
```

The dates are the same complex-date object events carry (an unknown one has `first: null`). The
places are the settlement detail shape of `GET /api/v2/settlements/<uuid>`.

Seen in the bundle but **not** confirmed or implemented: `birthSettlementName` (0 hits for a
plain town name), `birthSettlementGeorequisites`, `owner`, `hasAllTags[N]`, `privacy[N]`,
`settlement` / `parish` / `fund` / `register` / `case` / `surnameId`, `otherUser`, `bindAllowed`.
The facets, `/api/v3/persons/get-filters-data`, are **POST-only** (a GET is a 405) and not
implemented.

**Legacy:** `GET /api/v2/persons?names=…` is an older fuzzy search. It honours only `names` and a
legacy `type=regularPerson|catalogPerson`, wraps matched text in `<em>`, and silently ignores
every other filter (`sex`, years, `orderBy`). The `settlement=` list below is the same endpoint.

### Persons — authed read (Bearer)

- `GET /api/v2/profile` → `{ user:{uuid,email,…}, profile:{displayName,firstName,lastName,
  middleName,gender,…} }` — the current account.
- `GET /api/v2/tree` → the tree-canvas graph — see [Tree graph](#tree-graph-bearer) below.
- `GET /api/v2/persons/<uuid>` → the `regularPerson` view:
  `{ uuid, type:"regularPerson", displayName, originalDisplayName, shortDisplayName, ownerId,
  gender, birthPlace, deathPlace, deathSettlementText, photo, biography, isMine, isMe,
  canBuildTree, privacyType, updatedAt, tags, isGrantedToMe }`. `ownerId` (the owning account)
  is here but **not** on the public settlement list; `tags` is the person's «метки» (see the
  Tags sub-resource below).
- `GET /api/v2/persons/<uuid>/basic` → `{ uuid, createdAt, updatedAt, gender, privacy,
  firstName, lastName, middleName, birthLastName, birthFirstName }` — the edit-form source.
- `GET /api/v2/persons/<uuid>/events` → `[{ uuid, type, date, settlement, comment,
  participants, … }]`.

#### Missing persons

familio has no single answer for a person that does not exist. Each sub-resource answers it
differently, and two of them use **409**, the optimistic-lock status. These were confirmed live
on 2026-10-08 against three kinds of uuid: a person deleted from the account's tree, a random
uuid, and a catalog person (`type: catalogPerson`). The `/persons/<uuid>` namespace treats a
catalog person as missing too.

| `GET /api/v2/persons/<uuid>…` | missing person | client maps it to |
|---|---|---|
| (the regularPerson view) | **409** `{"message":"Персона не найдена","code":2604}` | `ErrNotFound` |
| `/basic` | 404 «Персона <uuid> не найдена», code 3 | `ErrNotFound` |
| `/biography` | 404 «Не найдена персона <uuid>», code 3 | `ErrNotFound` |
| `/sources` | **409** «Не найдена персона <uuid>», code **0** | `ErrNotFound` |
| `/events` | **200** `[]` — indistinguishable from "no events" | — (check `/basic`) |
| `/tags` | **403** «Нет доступа», code 1, the same as for another account's person | `ErrAccessDenied` |

So a 409 is `ErrNotFound` when its body reports the resource missing, meaning code 2604 or a
message containing «не найден». Any other 409 stays `ErrConflict`. A **malformed** uuid gets
400 «Некорректный запрос» / «Невалидный UUID персоны» on `/persons/<uuid>`, `/basic` and
`/tags`. On `/events`, `/biography` and `/sources` it gets **409** code 0 instead,
«Недопустимый идентификатор персоны» / «Невозможно получить значение». That 409 is left as
`ErrConflict`; the client never builds such a uuid itself.

The error bodies arrive `\u`-escaped (see Backend & conventions). They are shown decoded here.
- Frontend routes (`_buildManifest`): `/persons/new`, `/persons/new/simple/[id]`,
  `/persons/[personId]`, `/my-tree`, `/tree`, `/persons`.

### Tree graph (Bearer)

`GET /api/v2/tree` returns the whole tree-editor canvas in **one request**, centred on the
account's own person. Confirmed live 2026-07-30 (17 nodes on the test account).

```jsonc
{ "nodes": [ {
    "nodeId": "ee6f86f4-…-35befb3606dd.1",   // personUuid + a PLACEMENT SUFFIX — not a bare uuid
    "nodeParams": {
      "role": "parent",                       // central_person | parent | child | spouse
      "roleName": "Прадедушка",               // Russian kinship label vs. the central person
      "roleShortName": "Прадедушка", "roleNameAccusative": "прадедушку",
      "parents":  [ {"sex":"male","nodeId":"6ca2b100-….0"} ],  // OBJECTS, 0–2
      "partners": [ "45613c61-….1" ],                          // BARE node ids — asymmetric
      "layer": 3,                             // generational distance (0 = central person)
      "isPlaceholder": false,
      "hasMore": true,                        // more relatives exist OUTSIDE this response
      "isRecursionFound": false },
    "personData": {
      "personId": "ee6f86f4-…-35befb3606dd",  // the real person uuid
      "sex": "male", "lastName": "Мальчиков", "firstName": "Николай",
      "patronymic": "Васильевич",             // = /basic's middleName, renamed
      "birthFirstName": "", "birthLastName": "", "initials": "НМ",
      "photo": "/images/user_files/<owner>/persons/<uuid>.thumb-400x400.jpg",
      "locality": "Кириллово",
      "dateBirth": "29.11.1890 ст.",          // DISPLAY strings, not parseable — see below
      "dateDeath": "После 29.04.1926", "age": "", "hasDeathEvent": true,
      "isPrivate": false, "isMe": false, "isMine": true, "isUserPerson": false,
      "owner": "894dc7d5-…",
      "basicUpdatedAt": "…", "photoUpdatedAt": "…", "biographyUpdatedAt": "…" } } ],
  "edges": { "layoutBasis": [ {"source":"<parent nodeId>","target":"<child nodeId>"} ] },
  "theme": [], "recursiveNodes": [] }
```

Four traps:

- **`nodeId` is not a person uuid.** It is `<personUuid>.<n>`, because one person can be placed
  more than once in a layout. `personData.personId` is the uuid; the `parents`/`partners`/`edges`
  lists all speak **node ids**, so resolving an edge to a person means stripping the suffix.
- **The two edge lists have different shapes** — `parents` holds `{sex, nodeId}` objects, `partners`
  holds bare id strings. And there is **no children list**: a child is the inverse of a `parents`
  edge (`edges.layoutBasis` states the same links as parent→child pairs).
- **It is a window, not the whole tree.** `hasMore: true` marks a node with relatives the response
  omits (15 of 17 nodes on the test account). Do not treat the node set as "every person I have".
- **The dates are display strings**, day-first Russian, possibly qualified («После 29.04.1926») and
  marking Julian dates with **«ст.»** (`29.11.1890 ст.`). For structured dates read the person's
  `/events`.

`theme` and `recursiveNodes` were both `[]` on every account observed, so their element shapes are
unknown and this client does not model them.

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
entity plus a mutable free-text comment. The **comment edit is optimistic-locked** by the same
`X-Base-Version` header as `/basic` and `/biography` (its value is the source's own `updatedAt`);
a missing token is rejected with «Не указана дата-время последнего обновления источника».

- **List** `GET /api/v2/persons/<personUuid>/sources` → **200**, a JSON array of source objects.
- **Create** `POST /api/v2/persons/<personUuid>/sources` with the **write body**
  `{ "uuid": "<entityUuid>", "type": "<case|catalog_person>", "catalogKey": <null|string> }`
  → **200**, returns the full (enriched) source object. The write carries only those three
  fields; `name`/`requisites`/`years`/`comment` are server-derived/defaulted.
- **Edit (comment only)** `PATCH /api/v2/persons/<personUuid>/sources/<entityUuid>` with
  `Content-Type: application/ld+json`, body `{ "comment": "…" }` and header
  `X-Base-Version: <the source's own updatedAt>` → **200**, returns the updated object. Only
  `comment` is mutable; the reference (`uuid`/`type`/`catalogKey`) is fixed — changing it is a
  different source (delete + create). A missing/stale version → **400/409**.
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

### Change history sub-resource (Bearer, Familio Plus)

The **«История изменений»** feature (shipped 2026-07-08, the `/persons-changelog` page; also
reachable per person from the person-page ⋮ menu) is a **read-only audit log** of every edit to
the account's persons. It lives under the account owner's uuid — the same value as the JWT `uuid`
claim / `Client.AccountUUID`; requesting another owner's history → **403**. The UI is Plus-gated;
the API behavior for a non-Plus account is unverified (likely 403 too).

- **List** `GET /api/v2/persons/history/<ownerUuid>` → **200** `{data: […], pager: {…}}` (the
  same `pager {page, itemsPerPage, totalItems}` envelope as the public persons list).
  **`page`, `itemsPerPage`, `orderBy` and `orderDirection` are all required** — omitting any of
  them → **400**. `orderBy=id` is the only observed value; `orderDirection` is `desc` (UI
  «От новых к старым») or `asc`.
- **Filter facets** `POST /api/v2/persons/history/<ownerUuid>/get-filters-data` with a JSON body
  of the currently-applied filters (`{}` for none, or e.g. `{"operation":["update"]}`) → **200**
  with the per-facet value/label/count vocabularies: `authorFilter`, `operationFilter`,
  `personDataTypeFilter`, `personFilter` (+ `personFilterHasMore`), `causeFilter`.

**List query parameters** (all filters optional; array params use PHP-style brackets):

| param | example | meaning |
|---|---|---|
| `page`, `itemsPerPage` | `page=1&itemsPerPage=20` | required paging (UI offers 20/50/100) |
| `orderBy`, `orderDirection` | `orderBy=id&orderDirection=desc` | required sort |
| `text` | `text=Тюжин` | free-text search over the entries |
| `operation[]` | `operation[]=update` | `create` / `update` / `delete` |
| `cause[]` | `cause[]=initialization` | `user` (Пользователь) / `initialization` (system-saved) |
| `authorId[]` | `authorId[]=<userUuid>` | who edited; the zero uuid `00000000-…` is the system author |
| `personId[]` | `personId[]=<personUuid>` | limit to specific persons (the per-person view) |
| `date[from]`, `date[till]` | `date[from]=2026-07-01T00:00:00+02:00` | RFC3339 happened-at range. **Both or neither** — see below |
| `personDataType[N][personDataBlock]` | `personDataType[0][personDataBlock]=event` | `basic` / `event` / `source` / `biography` |
| `personDataType[N][eventType]` | `personDataType[0][eventType]=birth` | with block `event`: an event-type key |
| `personDataType[N][sourceType]` | `personDataType[0][sourceType]=case` | with block `source`: `register` / `case` / `catalog_person` |

**The date range is both-or-neither.** Either bound alone is rejected with **409**,
«Отсутствует параметр date[till]» or «… date[from]», with code 0. There is no open-ended range, so to
bound one side, pass a wide value for the other. familio accepts `1970-01-01T00:00:00Z` ..
`2100-01-01T00:00:00Z`, and even `0001-01-01T00:00:00Z` .. `9999-12-31T23:59:59Z`. Both return the
same total as no date filter. Confirmed live 2026-10-08. `HistoryFilter` does this itself: a zero
`From` or `Till` goes out as `0001-01-01T00:00:00Z` or `9999-12-31T23:59:59Z`.

**Entry (read shape):**
```jsonc
{ "record": {
    "id": 112528756,                       // numeric, monotonically increasing (the orderBy key)
    "happenedAt": "2026-07-14T22:08:21.843518+00:00",
    "cause": "user",                       // "user" | "initialization"
    "operation": "delete",                 // "create" | "update" | "delete"
    "personDataBlock": "event",            // "basic" | "event" | "source" | "biography"
    "changes": { /* block-shaped snapshot, see below */ } },
  "person": { "id": "<uuid>", "lastName": "…", "firstName": "…", "middleName": null,
              "birthLastName": null, "birthFirstName": null, "gender": "male" },
  "author": { "id": "<userUuid>", "displayName": "Мальчиков Д." } }
```
`changes` is the **snapshot of the block after the operation** (for `delete`, the state that was
removed) and its shape follows `personDataBlock`: `basic` → the flat basic fields incl. `privacy`;
`event` → an event-like object (`uuid`, `type`, `date`, `comment`, `settlement`, `participants`);
`biography` → `{text}`; `source` → the source read shape (`uuid`, `type`, `name`, `requisites`,
`years`, `catalog`, `comment`). **There is no before/after diff in the API** — the UI's
«Было — Стало» view is computed client-side by comparing an update with the previous record for
the same person+block, and no per-entry detail endpoint exists (expanding details fires no request).

### Matches sub-resource (Bearer)

The **«Совпадения»** feature (the `/profile/matches` page) is familio's duplicate-candidate
inbox — analogous to Geni's Merge Center. Once a month familio compares every person in the
account's tree against other users' **public** persons and against **record catalogs**
(«справочники»), scoring each candidate pair; the user then confirms or rejects it. It is **not**
Plus-gated: the free tier gets matches too, but only for public persons and a subset of catalogs
(«Без подписки Familio Plus ищем совпадения только для публичных персон и в некоторых
справочниках»). Like the change history, the collection lives under the account owner's uuid (the
JWT `uuid` claim / `Client.AccountUUID`).

All endpoints are **POST** (the reads included — the filter travels in the body), except the
legacy list.

- **List (paged)** `POST /api/v2/users/<ownerUuid>/matches/get-by-filters?page=&itemsPerPage=`
  → **200** `{data: […], pager: {page, itemsPerPage, totalItems}, dataVersionMark: "…"}`. Unlike
  the change-history list, **the paging params are optional** (the server defaults to `page=1`,
  `itemsPerPage=15`); there is no `orderBy`/`orderDirection`.
- **List (cursor)** `POST /api/v2/users/<ownerUuid>/matches/get-by-filters-scroll?pageAfterItem=&itemsPerPage=`
  → **200**, same shape but with the cursor envelope `pager: {lastItem, hasMore}`. Omit
  `pageAfterItem` for the first page, then pass the previous page's `lastItem`.
- **Filter facets** `POST /api/v2/users/<ownerUuid>/matches/get-filters-data` with the same filter
  body → **200** with `dateFilter`, `personFilter`, `userFilter`, `catalogFilter`, `statusFilter`,
  each `[{item: {value, displayValue}, count}]` — the same facet shape as the change history. Each
  facet is computed with the *other* filters applied (a facet does not narrow itself), so
  `statusFilter` ignores a posted `status`.
- **Set status** `POST /api/v2/users/<ownerUuid>/matches/{confirm,reject,undecide}-by-ids` with a
  **bare JSON array** of match uuids (`["95e794df-…"]`, *not* an object wrapping one) → **200**.
  `undecide-by-ids` returns matches to `undecided`, so confirm and reject are both reversible.
- **Set status filter-wide** `POST /api/v2/users/<ownerUuid>/matches/{confirm,reject}-by-filters`
  with `{"filter": {…, "excludeUuids": []}}` plus the header
  `X-Base-Version: <the list's dataVersionMark>` (the same optimistic-lock pattern as `/basic`,
  `/biography` and source comments). This is how the UI's «Все совпадения» bulk action applies a
  decision to every match matching the current filter, minus any deselected ones.
  **Not implemented by this client** — see the `status` caveat below.
- **Legacy list** `GET /api/v2/users/<ownerUuid>/matches?page=&itemsPerPage=` → **200**, the same
  response shape with no filtering. Superseded by `get-by-filters`.

**The filter body** — every one of the seven keys is **required**; omitting any is rejected with
**400** `{"type":"simple_error","message":"Неправильный формат фильтра <name>","code":4}`. Empty
lists mean "no restriction" and must be `[]`, not `null`:

| key | example | meaning |
|---|---|---|
| `person` | `["<personUuid>"]` | limit to specific persons **of your own** tree |
| `user` | `["<userUuid>"]` | limit to matches whose foreign person belongs to these users |
| `catalog` | `["vss"]` | limit to record catalogs («Источник совпадения»); keys from `catalogFilter` |
| `date` | `["2026-07-20"]` | the monthly batch that produced the match |
| `status` | `["undecided"]` | `undecided` (Ожидающие) / `confirmed` (Подтверждённые) / `rejected` (Отклонённые) |
| `minTotalScore`, `maxTotalScore` | `1` … `99` | the probability window the UI shows as «Вероятность: 1%-99%» |

`status` must be an **array** on the read endpoints — a scalar string is rejected with
**400** «Неправильный формат фильтра status». The bulk `*-by-filters` write body captured from the
UI nevertheless sends it as a **scalar** (`"status": "undecided"`). That asymmetry could not be
verified without mass-mutating real matches, which is why the bulk writes are documented here but
deliberately left unimplemented; the `*-by-ids` endpoints cover the same ground safely.

**Match (read shape):**
```jsonc
{ "uuid": "95e794df-…",     // the MATCH's id — what the *-by-ids endpoints take (not a person uuid)
  "score": 99,               // «Вероятность совпадения», percent
  "date": "2026-07-20",      // the monthly batch that produced it
  "status": "undecided",     // "undecided" | "confirmed" | "rejected"
  "ownPerson":     { /* your person — always a regularPerson */ },
  "foreignPerson": { /* the candidate duplicate — polymorphic, see below */ },
  "detailedScore": { "firstName": 20, "lastName": 20, "middleName": 20,
                     "birthDate": 10, "deathDate": 0, "birthPlace": 10 } }
```
`detailedScore` is a per-field points breakdown that **does not sum to `score`** (observed 80 vs a
score of 99, and 70 vs a score of 3) — `score` is a separate probability.

Both person sides use the **persons read shape** (the same one `GET /api/v2/persons` returns, i.e.
this client's `Person`) plus the ownership fields `ownerId`, `isMine`, `isGrantedToMe`,
`privacyType`, `biography`, and full `birthPlace`/`deathPlace` settlement objects. `foreignPerson`
is **polymorphic**, discriminated by `type`:
- **`regularPerson`** — a person in another user's tree: carries `ownerId` (their user uuid,
  linkable as `/users/<uuid>`), privacy, tags and places.
- **`catalogPerson`** — a record-catalog entry: carries `catalogKey` / `catalogName` plus
  `updating`, and **none** of the ownership/place/privacy fields.

`dataVersionMark` (e.g. `"2026-07-27T08:23:44+00:00"`) stamps the match set the page was computed
from; it is only needed as `X-Base-Version` on the bulk filter-wide writes.

### Tags sub-resource (Bearer, Familio Plus)

**«Метки»** (the `/profile/my-tags` page) are the account's own coloured labels, attached to
persons to group them — "Метки помогают группировать записи о персонах и быстрее находить нужных
людей". A tag belongs to the account that created it, and **only a person's author may manage that
person's tags**.

Two collections, and they are *not* siblings: the tag catalogue is a **top-level `/tags`**
resource (listed via the account owner's uuid), while the person↔tag links live under
`/persons/<uuid>/tags`.

| Method | Path | Body | Purpose |
|---|---|---|---|
| `GET` | `/api/v2/users/<ownerUuid>/tags` | — | the account's tags; **unpaged bare array** |
| `POST` | `/api/v2/tags` | `{tag,color,description}` | create → the new tag, with its `id` |
| `PUT` | `/api/v2/tags/<tagId>` | `{tag,color,description}` | update → the refreshed tag |
| `DELETE` | `/api/v2/tags/<tagId>` | — | delete (unassigns it from every person) |
| `POST` | `/api/v2/tags/get-by-persons-id-list` | `[personUuid]` | bulk: tags per person |
| `GET` | `/api/v2/persons/<personUuid>/tags` | — | one person's assigned tags |
| `POST` | `/api/v2/persons/<personUuid>/tags` | `[tagId]` | assign → **200** + the person's refreshed tag list |
| `DELETE` | `/api/v2/persons/<personUuid>/tags` | `[tagId]` | unassign → **204**, empty body |

Both person-link writes take a **bare JSON array** of tag ids, like the matches `*-by-ids`
endpoints — but they are **asymmetric in what they return**: assign echoes the person's whole
refreshed tag list, unassign answers 204 with nothing (re-read `GET …/tags` if you need it).
Assign **adds**; it never replaces the person's set, and re-assigning an already-assigned tag is a
no-op. Note there is **no `X-Base-Version`** on any tags call — unlike `/basic`, `/biography` and
source comments, tags carry no optimistic lock.

**Tag (read shape):**
```jsonc
{ "id": 2832,                       // an INTEGER — tags are the one resource not keyed by a uuid
  "tag": "Проверить в архиве",      // «Название»; the writable field is `tag`, not `name`
  "color": "mint-mist",             // a palette CODE, not a hex value — see below
  "description": "Нужен запрос в ЦГА",
  "isFree": true }                  // server-computed; see the Plus gating below
```

`id` being a small **integer** rather than a uuid is the trap worth remembering: every other
familio identifier in this document is a uuid string, so a `string`-typed field here fails to
decode (`cannot unmarshal number into … of type string`). The ids are sequential and
account-global.

**`color` is a palette code.** familio never accepts or returns a hex; the UI maps seven fixed
codes to pastel fills:

| code | fill | code | fill |
|---|---|---|---|
| `rose-mist` | `#FFEBEB` | `ice-blue` | `#EBFEFF` |
| `soft-peach` | `#FFF4EB` | `lavender-haze` | `#EBEBFF` |
| `lemon-tint` | `#FDFFEB` | `lilac-glow` | `#FAEBFF` |
| `mint-mist` | `#EBFFEB` | | |

**Validation the web editor applies before it will submit** (mirrored by this client's
`TagInput.Validate`): `tag` is required and trimmed, ≤ **1000** characters, and must be unique
**case-insensitively** among the account's tags (`«Метка с таким текстом уже создана»`); `color` is
required; `description` is optional, ≤ **5000** characters. The uniqueness rule needs the whole
list, so this client leaves it to the caller/server.

**Familio Plus gating.** Tags are a Plus feature. On a non-Plus account only the tags flagged
`isFree: true` are usable, and **at most one** of them — the UI shows «Доступно с подпиской
Familio Plus … только одна метка» and renders the rest disabled rather than hiding them. So the
list read can return tags a free account cannot actually assign. This is UI-side policy; the API
returns everything and this client surfaces `IsFree` without enforcing anything.

**The bulk read's empty-map trap.** `get-by-persons-id-list` returns a **map keyed by person
uuid** — `{"<personUuid>": [ {tag}, … ]}` — but the PHP backend serializes an *empty*
associative array as **`[]`**, not `{}`. A stock Go map decoder rejects that, hence
`PersonTags.UnmarshalJSON`. Persons with no tags may also simply be absent from a non-empty map.

**The `regularPerson` view's `tags` are bare ids.** `GET /persons/<uuid>` returns
`"tags": [2832]` — integer `Tag.id` values, *not* tag objects (this client exposes them as
`RegularRecord.Tags []int`). Resolve them against the account's list, or read the full objects from
`GET /persons/<uuid>/tags`.

There is **no tag facet on the persons list** — `GET /api/v2/persons` takes no tag filter, so tags
are read per person or in bulk, never used as a search dimension.

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

## Client coverage — what this library deliberately does not implement

Everything documented above is implemented by `go-familio` **except** the
following. These are choices, not oversights; each is additive, so any of them can
arrive in a minor release without breaking the v1 surface.

| Not implemented | Why | Consequence |
|---|---|---|
| Source **catalog browsing** — `/api/v2/{organizations,funds,registers,cases}` and `GET /persons?type=catalogPerson&names=…&bindAllowed=true` | the drill-down UI flow is several endpoints deep and only needed to *discover* a reference | `CreateSource` needs a reference uuid obtained elsewhere (a browser session) |
| **Photo** — `POST`/`DELETE /persons/<uuid>/photo` | binary upload; no consumer needs it yet | `CreatePerson` always sends `photo: null` |
| `PUT /api/v2/validate/complex-date`, `POST /api/v2/surnames/validate` | the client builds dates from a typed `DateRange`, so a server round-trip buys little | invalid dates surface as a 400 on the real write instead of ahead of it |
| Matches bulk `{confirm,reject}-by-filters` | the filter body's `status` is a **scalar** here but an **array** on the reads, and confirming that asymmetry means mass-mutating real matches | use the `*-by-ids` endpoints, which cover the same ground safely |
| A tag facet / name search on `GET /persons` | familio has none | tags are read per person or in bulk, never as a search dimension |
| Person search **facets** — `POST /api/v3/persons/get-filters-data`, and the unconfirmed search filters (owner, tags, privacy, places, archive refs) | the search itself covers finding a person by name and dates | narrow by name, type, gender and dates; the rest via `DoRaw` |

Any endpoint, documented here or not, can still be reached through `Client.DoRaw` (the CLI's
`familio api`). It sends a raw request with the client's bearer, rate limit and retry, and
returns the response undecoded.

`ListSettlementPersons` also pages the whole settlement into memory (~20 k rows
for a large one) with no caller-side limit.

## Known limitations & open questions

1. **Wedding events don't upsert** — re-POSTing a `wedding` duplicates it (the upsert trick is
   confirmed only for single-subject `birth`/`death`). So `familio_marriage` edits its date/comment
   in place by **delete-old + create-new** (the new event gets a new uuid), exactly like the
   christening; `partners` stays RequiresReplace by design (a different pair is a different marriage).
2. **`PUT …/events/<id>`** — blocked by an unknown concurrency-token field name; not needed
   while the POST-upsert covers births/deaths and delete+create covers weddings.
3. **Token refresh** — the JWT lasts ~30 days and there is no mint endpoint. The `t` cookie's own
   value is a usable JWT, so the client sends that when it is valid and falls back to re-scraping
   `__NEXT_DATA__.token` (an opaque cookie, a malformed token, or one near expiry).

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

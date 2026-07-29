# familio CLI

`familio` is a command-line client for the
[familio.org](https://familio.org) genealogy API — a thin façade over the
[`go-familio`](../../) library for quick lookups (and a few targeted writes)
without writing Go. All output is JSON on stdout, and every uuid printed in
machine-readable output is a **full** uuid (never a truncated prefix).

## Install

```bash
go install github.com/dmalch/go-familio/cmd/familio@latest
```

## Authentication

Most commands need a logged-in familio.org session. Credentials are resolved
in this order (matching the Terraform provider):

1. `-cookies <header>` flag, or `FAMILIO_COOKIES` — a raw
   `name=value; name=value` cookie header copied from your browser's DevTools
   Network panel.
2. `FAMILIO_SESSION` — the bare `t` session-cookie value.
3. `-browser <name>` flag, or `FAMILIO_BROWSER` — read cookies straight from a
   logged-in browser on this host (`chrome`, `edge`, `brave`, `chromium`,
   `vivaldi`, `opera`, `firefox`, `safari`). On macOS this may prompt for Full
   Disk Access.

The `settlement` commands hit a public endpoint and need no credentials.

Global flags (`-cookies`, `-browser`) may appear **before or after** the
command and its arguments — `familio person get <uuid> -browser chrome` works.

## Commands

```bash
familio whoami                       # print the authenticated account uuid
familio person get <uuid>            # record + derived relations + birth/death years + events
familio person set-biography <uuid>  # set a biography from -text or stdin (-append to keep existing)
familio tree <uuid>                  # crawl connected persons with structured relations
familio marriage create <a> <b>      # link two persons with a wedding event
familio marriage delete <p> <union>  # delete a marriage (union) by a participant + union uuid
familio settlement get <uuid>        # a settlement (place) record
familio settlement persons <uuid>    # persons tied to a settlement (public)
familio sources list <person-uuid>   # a person's source citations
familio history list                 # change-history entries (Familio Plus)
familio history filters              # change-history facets with counts (Familio Plus)
familio matches list                 # duplicate-candidate matches («Совпадения»)
familio matches filters              # matches facets with counts
familio matches confirm <uuid>…      # confirm matches by match uuid
familio matches reject <uuid>…       # reject matches by match uuid
familio matches undecide <uuid>…     # undo a confirm/reject
familio help                         # full command list
```

### `person get`

Beyond the raw `basic` record and `events`, the response adds a derived
convenience view: `relations` (`parents`/`spouses`/`children`, each
`{uuid, name}`), top-level `birthYear`/`deathYear` and `birthDate`/`deathDate`,
and — on each spouse — the `marriageUuid` (the underlying wedding-event/union
uuid needed to import a `familio_marriage` or target it for deletion).

### `tree`

Crawls the persons connected to a root uuid and prints them as a JSON array of
`{uuid, name, year, parents, spouses, children}` nodes — the "familio as ground
truth" foundation that replaces hand-written BFS crawlers.

```bash
familio tree <uuid> [-up | -down | -component] [-surname <s>] [-depth <n>]
```

- `-up` follows parents (ancestors); `-down` follows children (descendants);
  `-component` (default) walks the whole connected component.
- `-surname <s>` only expands through people with that last name — the way to
  keep a crawl from pulling living in-law branches. Non-matching people are
  still emitted, just not expanded.
- `-depth <n>` caps the BFS distance from the root (`0` = unlimited).

### `history list` / `history filters`

`history list` pages through the account's **«История изменений»** (person
change history — a Familio Plus feature): every create/update/delete of your
persons' basic data, events, sources, and biographies, newest first. Each
entry is `{record, person, author}`; `record.changes` is the affected block's
snapshot after the operation (the API carries no before/after diff).

```bash
familio history list [-person <uuid>] [-author <uuid>] [-operation create|update|delete] \
  [-cause user|initialization] [-block basic|event|source|biography] \
  [-event-type <t>] [-source-type <t>] [-text <s>] \
  [-from <date>] [-till <date>] [-page <n>] [-limit <n>] [-asc]
```

- `-person`, `-author`, `-operation`, and `-cause` are repeatable;
  `-event-type`/`-source-type` narrow a `-block event`/`-block source` filter.
- `-from`/`-till` accept `YYYY-MM-DD` (local day bounds) or full RFC3339.
- One page per call (`-page`/`-limit`); the `pager.totalItems` in the output
  tells you when to stop.

`history filters` prints the facet vocabularies (who edited, operations,
causes, data types, persons) with per-value entry counts — useful for
discovering what's in the log before filtering.

### `matches list` / `matches filters`

`matches list` pages through the account's **«Совпадения»** — familio's
duplicate-candidate inbox. Once a month familio pairs each of your persons with
other users' *public* persons and with record-catalog entries, scoring the pair;
you then confirm or reject it. Each entry carries the match `uuid` (the handle
the decision commands take — **not** a person uuid), the `score` percentage,
a `detailedScore` per-field breakdown, and both sides as `ownPerson` /
`foreignPerson`.

```bash
familio matches list [-status undecided|confirmed|rejected] [-person <uuid>] [-user <uuid>] \
  [-catalog <key>] [-date YYYY-MM-DD] [-min-score <n>] [-max-score <n>] \
  [-page <n>] [-limit <n>] [-all]
```

- `-status`, `-person`, `-user`, `-catalog` and `-date` are repeatable.
  `-person` filters by *your* persons; `-user` by the owner of the matched
  person; `-catalog` by record catalog (keys come from `matches filters`).
- `-min-score`/`-max-score` bound the probability window (1–99), the same
  control the UI labels «Вероятность».
- One page per call by default (`-page`/`-limit`, `pager.totalItems` tells you
  when to stop), or `-all` to sweep every page through the scroll cursor — that
  prints a bare JSON array with no `pager`.

`foreignPerson.type` tells you what the match is against: `regularPerson` (a
person in another user's tree, with `ownerId`) or `catalogPerson` (a
record-catalog entry, with `catalogKey`/`catalogName`). Filter client-side
rather than adding flags, e.g. only catalog matches:

```bash
familio matches list -all | jq '[.[] | select(.foreignPerson.type == "catalogPerson")]'
```

`matches filters` prints the facet vocabularies (batch dates, your persons,
foreign owners, catalogs, statuses) with per-value match counts. The same
filter flags narrow the counts, mirroring the UI's faceted search.

### `matches confirm` / `matches reject` / `matches undecide`

Set the status of one or more matches by **match uuid**:

```bash
familio matches reject   [-yes] <match-uuid> [<match-uuid>…]
familio matches confirm  [-yes] <match-uuid> [<match-uuid>…]
familio matches undecide [-yes] <match-uuid> [<match-uuid>…]
```

Each lists the affected matches and prompts `[y/N]` on stderr before acting
(stdout stays pure JSON); `-yes` skips the prompt for scripted use. Answering
anything but `y`/`yes` — including an empty or closed stdin — aborts without
issuing a request.

Nothing here is destructive: `matches undecide` returns a match to the
`undecided` state, undoing either decision.

## Examples

```bash
# Public — no auth:
familio settlement get 1f8c…uuid
familio settlement persons 1f8c…uuid

# Authed:
export FAMILIO_COOKIES='t=eyJ…; other=…'
familio whoami
familio person get 3a2b…uuid
familio tree 3a2b…uuid -up -surname Иванов
familio sources list 3a2b…uuid
familio history list -operation update -from 2026-07-01
familio matches list -status undecided -min-score 90
familio matches filters

# Writes (real mutations on your account):
familio matches reject -yes <match-uuid>      # undo with: familio matches undecide <match-uuid>
familio marriage create 3a2b…uuid 9f0e…uuid -date 1850-06-12 -comment "венчание"
familio marriage delete 3a2b…uuid <union-uuid>
echo "Жил-был человек." | familio person set-biography 3a2b…uuid
```

Exit codes: `0` success, `1` command error, `2` usage error.

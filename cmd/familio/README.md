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

# Writes (real mutations on your account):
familio marriage create 3a2b…uuid 9f0e…uuid -date 1850-06-12 -comment "венчание"
familio marriage delete 3a2b…uuid <union-uuid>
echo "Жил-был человек." | familio person set-biography 3a2b…uuid
```

Exit codes: `0` success, `1` command error, `2` usage error.

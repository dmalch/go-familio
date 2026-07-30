package main

import (
	"context"
	"fmt"
	"io"
	"sort"
)

// commandTree returns the CLI command tree. Top-level entries are either
// flat commands (run set) or resource groups (sub set). It is a function
// rather than a package variable to avoid an initialization cycle:
// runHelp -> printUsage -> the tree.
func commandTree() map[string]*command {
	return map[string]*command{
		"whoami": {summary: "show the authenticated account (uuid, email, display name)", run: runWhoami},
		"help":   {summary: "show this usage text", run: runHelp},

		"tree":  {summary: "crawl connected persons with structured relations ([-up|-down|-component] [-surname s] [-depth n])", run: runTree},
		"graph": {summary: "print the account's whole tree graph (node ids + parent/partner edges) in one request", run: runGraph},

		"person": {summary: "person resource", sub: map[string]*command{
			"get":           {summary: "fetch a person's record, relations, years, and events by uuid", run: runPersonGet},
			"set-biography": {summary: "set (or -append) a person's biography from -text or stdin", run: runPersonSetBiography},
		}},
		"marriage": {summary: "marriage (wedding event) resource", sub: map[string]*command{
			"create": {summary: "link two persons with a wedding event ([-date] [-comment])", run: runMarriageCreate},
			"delete": {summary: "delete a marriage by <person-uuid> <union-uuid>", run: runMarriageDelete},
		}},
		"settlement": {summary: "settlement (place) resource", sub: map[string]*command{
			"get":     {summary: "fetch a settlement by uuid", run: runSettlementGet},
			"persons": {summary: "list the persons tied to a settlement (public, no auth)", run: runSettlementPersons},
		}},
		"sources": {summary: "person source citations", sub: map[string]*command{
			"list": {summary: "list a person's source citations by person uuid", run: runSourcesList},
		}},
		"matches": {summary: "candidate duplicate persons («Совпадения»)", sub: map[string]*command{
			"list":     {summary: "list match candidates ([-status s] [-person u] [-user u] [-catalog k] [-date d] [-min-score n] [-page n|-all] …)", run: runMatchesList},
			"filters":  {summary: "show the matches filter facets (dates, persons, users, catalogs, statuses) with counts", run: runMatchesFilters},
			"confirm":  {summary: "confirm matches by <match-uuid>… ([-yes]; reversible with \"matches undecide\")", run: runMatchesConfirm},
			"reject":   {summary: "reject matches by <match-uuid>… ([-yes]; reversible with \"matches undecide\")", run: runMatchesReject},
			"undecide": {summary: "return matches to the undecided state by <match-uuid>… ([-yes])", run: runMatchesUndecide},
		}},
		"tags": {summary: "person tags («метки», Familio Plus)", sub: map[string]*command{
			"list":       {summary: "list the tags this account owns («Мои метки»)", run: runTagsList},
			"person":     {summary: "list the tags assigned to a person by <person-uuid>", run: runTagsPerson},
			"by-persons": {summary: "list the tags of several persons by <person-uuid>…, keyed by person", run: runTagsByPersons},
			"colors":     {summary: "show the accepted palette colour codes and their hexes (no network call)", run: runTagsColors},
			"create":     {summary: "create a tag (-name s -color c [-description s])", run: runTagsCreate},
			"update":     {summary: "replace a tag's fields by <tag-id> (-name s -color c [-description s])", run: runTagsUpdate},
			"delete":     {summary: "delete tags by <tag-id>… ([-yes]; unassigns them from every person)", run: runTagsDelete},
			"assign":     {summary: "assign tags to a person: <person-uuid> <tag-id>… ([-yes])", run: runTagsAssign},
			"unassign":   {summary: "unassign tags from a person: <person-uuid> <tag-id>… ([-yes])", run: runTagsUnassign},
		}},
		"history": {summary: "person change history (Familio Plus)", sub: map[string]*command{
			"list":    {summary: "list change-history entries ([-person u] [-operation op] [-block b] [-from d] [-till d] [-text s] [-page n] …)", run: runHistoryList},
			"filters": {summary: "show the history filter facets (authors, operations, data types, …) with counts", run: runHistoryFilters},
		}},
	}
}

// printUsage writes the command list to w.
func printUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, "familio — command-line client for the familio.org API\n\n"+
		"Usage:\n  familio <command> [<subcommand>] [args] [flags]\n\n"+
		"Global flags may appear before or after the command and its arguments.\n\nCommands:\n")
	printCommands(w, "", commandTree())
	_, _ = fmt.Fprint(w, "\nGlobal flags:\n"+
		"  -cookies <header>   session cookies as a raw \"name=value; …\" header (or set FAMILIO_COOKIES)\n"+
		"  -browser <name>     read cookies from a logged-in browser (or set FAMILIO_BROWSER)\n\n"+
		"Auth precedence: -cookies/FAMILIO_COOKIES > FAMILIO_SESSION > -browser/FAMILIO_BROWSER.\n"+
		"The settlement commands are public and need no credentials.\n")
}

// printCommands recursively walks the command tree printing one line per
// leaf, with the full dotted path. Internal-only nodes (those with
// sub != nil) collapse into their leaves.
func printCommands(w io.Writer, prefix string, sub map[string]*command) {
	names := make([]string, 0, len(sub))
	for n := range sub {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		c := sub[n]
		path := n
		if prefix != "" {
			path = prefix + " " + n
		}
		if c.sub == nil {
			_, _ = fmt.Fprintf(w, "  %-26s %s\n", path, c.summary)
			continue
		}
		printCommands(w, path, c.sub)
	}
}

// runWhoami prints the account that owns the active session: its uuid, the
// address it logs in with, and the account holder's name.
func runWhoami(ctx context.Context, g *globalOpts, _ []string) error {
	c, err := newClient(g)
	if err != nil {
		return err
	}
	profile, err := c.GetProfile(ctx)
	if err != nil {
		return err
	}
	return render(g.stdout, profile)
}

// runGraph prints the account's whole tree as familio's editor loads it — one
// request, node ids plus parent/partner edges, no names or dates. Use
// "familio tree <uuid>" for a rooted crawl with names, years and relations.
func runGraph(ctx context.Context, g *globalOpts, _ []string) error {
	c, err := newClient(g)
	if err != nil {
		return err
	}
	graph, err := c.GetTreeGraph(ctx)
	if err != nil {
		return err
	}
	return render(g.stdout, graph)
}

// runHelp prints the usage text to stdout.
func runHelp(_ context.Context, g *globalOpts, _ []string) error {
	printUsage(g.stdout)
	return nil
}

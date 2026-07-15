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
		"whoami": {summary: "show the authenticated account uuid", run: runWhoami},
		"help":   {summary: "show this usage text", run: runHelp},

		"tree": {summary: "crawl connected persons with structured relations ([-up|-down|-component] [-surname s] [-depth n])", run: runTree},

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

// runWhoami prints the uuid of the account that owns the active session.
func runWhoami(ctx context.Context, g *globalOpts, _ []string) error {
	c, err := newClient(g)
	if err != nil {
		return err
	}
	uuid, err := c.AccountUUID(ctx)
	if err != nil {
		return err
	}
	return render(g.stdout, map[string]string{"uuid": uuid})
}

// runHelp prints the usage text to stdout.
func runHelp(_ context.Context, g *globalOpts, _ []string) error {
	printUsage(g.stdout)
	return nil
}

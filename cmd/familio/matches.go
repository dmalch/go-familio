package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"

	familio "github.com/dmalch/go-familio"
)

// matchStatuses are the accepted -status values, in the order the usage text
// lists them.
var matchStatuses = []string{
	familio.MatchStatusUndecided,
	familio.MatchStatusConfirmed,
	familio.MatchStatusRejected,
}

// matchesListFlags carries the raw "matches list" flag values before they are
// validated into a familio.MatchFilter.
type matchesListFlags struct {
	persons  multiFlag
	users    multiFlag
	catalogs multiFlag
	dates    multiFlag
	statuses multiFlag
	minScore int
	maxScore int
	page     int
	limit    int
	all      bool
}

// register wires the matches list flags onto fs.
func (m *matchesListFlags) register(fs *flag.FlagSet) {
	fs.Var(&m.persons, "person", "filter by one of your person uuids (repeatable)")
	fs.Var(&m.users, "user", "filter by the matched person's owner user uuid (repeatable)")
	fs.Var(&m.catalogs, "catalog", "filter by record-catalog key, e.g. vss (repeatable; see \"matches filters\")")
	fs.Var(&m.dates, "date", "filter by match batch date, YYYY-MM-DD (repeatable)")
	fs.Var(&m.statuses, "status", "filter by status: "+strings.Join(matchStatuses, "|")+" (repeatable)")
	fs.IntVar(&m.minScore, "min-score", 1, "lowest match probability percent to include (1-99)")
	fs.IntVar(&m.maxScore, "max-score", 99, "highest match probability percent to include (1-99)")
	fs.IntVar(&m.page, "page", 0, "1-based page number (default 1; not allowed with -all)")
	fs.IntVar(&m.limit, "limit", 15, "matches per page")
	fs.BoolVar(&m.all, "all", false, "page through every match via the scroll cursor")
}

// filter validates the raw flags into a familio.MatchFilter.
func (m *matchesListFlags) filter() (familio.MatchFilter, error) {
	f := familio.MatchFilter{
		PersonIDs:    m.persons,
		UserIDs:      m.users,
		Catalogs:     m.catalogs,
		Dates:        m.dates,
		Statuses:     m.statuses,
		MinScore:     m.minScore,
		MaxScore:     m.maxScore,
		Page:         m.page,
		ItemsPerPage: m.limit,
	}
	for _, s := range m.statuses {
		if !slices.Contains(matchStatuses, s) {
			return f, fmt.Errorf("invalid -status %q (want one of: %s)", s, strings.Join(matchStatuses, ", "))
		}
	}
	if m.minScore < 1 || m.minScore > 99 || m.maxScore < 1 || m.maxScore > 99 {
		return f, errors.New("-min-score/-max-score must be between 1 and 99")
	}
	if m.minScore > m.maxScore {
		return f, errors.New("-min-score must not exceed -max-score")
	}
	if m.all && m.page != 0 {
		return f, errors.New("-all cannot be combined with -page")
	}
	return f, nil
}

// runMatchesList lists match candidates («Совпадения»), one page per call, with
// the UI's filters exposed as flags. With -all it sweeps every page via the
// scroll cursor and prints a bare array instead of a paged envelope.
func runMatchesList(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("matches list")
	var m matchesListFlags
	m.register(fs)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) != 0 {
		return errors.New("matches list takes no positional arguments")
	}
	filter, err := m.filter()
	if err != nil {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}

	if m.all {
		all, err := scrollAllMatches(ctx, c, filter)
		if err != nil {
			return err
		}
		return render(g.stdout, all)
	}

	page, err := c.ListMatches(ctx, filter)
	if err != nil {
		return err
	}
	return render(g.stdout, page)
}

// scrollAllMatches walks the cursor endpoint until it reports no more pages.
func scrollAllMatches(ctx context.Context, c *familio.Client, filter familio.MatchFilter) ([]familio.Match, error) {
	all := []familio.Match{}
	for after := ""; ; {
		page, err := c.ScrollMatches(ctx, filter, after)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if !page.Pager.HasMore || page.Pager.LastItem == "" || len(page.Data) == 0 {
			return all, nil
		}
		after = page.Pager.LastItem
	}
}

// runMatchesFilters prints the matches facet vocabularies (dates, persons,
// users, catalogs, statuses) with their match counts. The same filter flags as
// "matches list" narrow the counts, mirroring the UI's faceted search.
func runMatchesFilters(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("matches filters")
	var m matchesListFlags
	m.register(fs)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) != 0 {
		return errors.New("matches filters takes no positional arguments")
	}
	filter, err := m.filter()
	if err != nil {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	filters, err := c.GetMatchFilters(ctx, filter)
	if err != nil {
		return err
	}
	return render(g.stdout, filters)
}

// runMatchesConfirm marks matches as confirmed by match uuid.
func runMatchesConfirm(ctx context.Context, g *globalOpts, args []string) error {
	return runMatchDecision(ctx, g, args, "confirm", familio.MatchStatusConfirmed,
		(*familio.Client).ConfirmMatches)
}

// runMatchesReject marks matches as rejected by match uuid.
func runMatchesReject(ctx context.Context, g *globalOpts, args []string) error {
	return runMatchDecision(ctx, g, args, "reject", familio.MatchStatusRejected,
		(*familio.Client).RejectMatches)
}

// runMatchesUndecide returns matches to the undecided state, undoing an earlier
// confirm or reject.
func runMatchesUndecide(ctx context.Context, g *globalOpts, args []string) error {
	return runMatchDecision(ctx, g, args, "undecide", familio.MatchStatusUndecided,
		(*familio.Client).UndecideMatches)
}

// runMatchDecision is the shared body of the three status commands: parse the
// match uuids, confirm the change with the user unless -yes was given, apply
// it, and report the new status. apply is one of the client's status methods,
// passed as a method expression.
func runMatchDecision(ctx context.Context, g *globalOpts, args []string,
	verb, status string, apply func(*familio.Client, context.Context, []string) error) error {
	fs := g.newFlagSet("matches " + verb)
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	ids, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(ids) == 0 {
		return errors.New("expected at least one <match-uuid> argument")
	}

	if !*yes && !confirmAction(g, verb, ids) {
		return errors.New(verb + " aborted")
	}

	c, err := newClient(g)
	if err != nil {
		return err
	}
	if err := apply(c, ctx, ids); err != nil {
		return err
	}
	return render(g.stdout, map[string]any{"status": status, "matches": ids})
}

// confirmAction prompts on stderr before a mutation and reports whether the
// user answered yes. Anything other than "y"/"yes" declines, and so does a
// failed read (EOF on a closed or empty stdin), so the safe answer is always
// the default.
func confirmAction(g *globalOpts, verb string, ids []string) bool {
	_, _ = fmt.Fprintf(g.stderr, "%s %d match(es):\n", strings.ToUpper(verb[:1])+verb[1:], len(ids))
	for _, id := range ids {
		_, _ = fmt.Fprintf(g.stderr, "  %s\n", id)
	}
	_, _ = fmt.Fprint(g.stderr, "Proceed? [y/N]: ")

	line, err := bufio.NewReader(g.stdin).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

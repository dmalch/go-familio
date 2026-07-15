package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"time"

	familio "github.com/dmalch/go-familio"
)

// multiFlag collects a repeatable string flag ("-person a -person b").
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// historyListFlags carries the raw "history list" flag values before they are
// validated into a familio.HistoryFilter.
type historyListFlags struct {
	persons    multiFlag
	authors    multiFlag
	operations multiFlag
	causes     multiFlag
	block      string
	eventType  string
	sourceType string
	text       string
	from       string
	till       string
	page       int
	limit      int
	asc        bool
}

// register wires the history list flags onto fs.
func (h *historyListFlags) register(fs *flag.FlagSet) {
	fs.Var(&h.persons, "person", "filter by person uuid (repeatable)")
	fs.Var(&h.authors, "author", "filter by editor user uuid (repeatable; 00000000-… = system)")
	fs.Var(&h.operations, "operation", "filter by operation: create|update|delete (repeatable)")
	fs.Var(&h.causes, "cause", "filter by cause: user|initialization (repeatable)")
	fs.StringVar(&h.block, "block", "", "filter by data block: basic|event|source|biography")
	fs.StringVar(&h.eventType, "event-type", "", "with -block event: the event type key (birth, wedding, …)")
	fs.StringVar(&h.sourceType, "source-type", "", "with -block source: register|case|catalog_person")
	fs.StringVar(&h.text, "text", "", "free-text search over the entries")
	fs.StringVar(&h.from, "from", "", "happened-at range start (YYYY-MM-DD or RFC3339)")
	fs.StringVar(&h.till, "till", "", "happened-at range end (YYYY-MM-DD or RFC3339)")
	fs.IntVar(&h.page, "page", 1, "1-based page number")
	fs.IntVar(&h.limit, "limit", 20, "entries per page")
	fs.BoolVar(&h.asc, "asc", false, "oldest first instead of newest first")
}

// filter validates the raw flags into a familio.HistoryFilter.
func (h *historyListFlags) filter() (familio.HistoryFilter, error) {
	f := familio.HistoryFilter{
		Text:         h.text,
		Operations:   h.operations,
		Causes:       h.causes,
		AuthorIDs:    h.authors,
		PersonIDs:    h.persons,
		Page:         h.page,
		ItemsPerPage: h.limit,
		Ascending:    h.asc,
	}
	if h.block == "" && (h.eventType != "" || h.sourceType != "") {
		return f, errors.New("-event-type/-source-type need -block")
	}
	if h.block != "" {
		f.DataTypes = []familio.HistoryDataType{{
			PersonDataBlock: h.block,
			EventType:       h.eventType,
			SourceType:      h.sourceType,
		}}
	}
	var err error
	if f.From, err = parseHistoryDate(h.from, false); err != nil {
		return f, err
	}
	if f.Till, err = parseHistoryDate(h.till, true); err != nil {
		return f, err
	}
	return f, nil
}

// parseHistoryDate parses a -from/-till value: RFC3339, or a bare local date
// that expands to the day's start (or, for -till, its end). Empty stays zero
// (unbounded).
func parseHistoryDate(v string, endOfDay bool) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	t, err := time.ParseInLocation("2006-01-02", v, time.Local)
	if err != nil {
		return time.Time{}, errors.New("invalid date " + v + ": want YYYY-MM-DD or RFC3339")
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Second)
	}
	return t, nil
}

// historyPageView mirrors familio.HistoryPage for rendering, with each raw
// changes snapshot decoded so its Cyrillic prints readably instead of as the
// server's \u-escaped bytes.
type historyPageView struct {
	Data  []historyEntryView `json:"data"`
	Pager familio.Pager      `json:"pager"`
}

type historyEntryView struct {
	Record historyRecordView        `json:"record"`
	Person familio.HistoryPersonRef `json:"person"`
	Author familio.HistoryAuthor    `json:"author"`
}

type historyRecordView struct {
	ID              int64  `json:"id"`
	HappenedAt      string `json:"happenedAt"`
	Cause           string `json:"cause"`
	Operation       string `json:"operation"`
	PersonDataBlock string `json:"personDataBlock"`
	Changes         any    `json:"changes"`
}

// buildHistoryView decodes each entry's raw changes for display; undecodable
// changes fall back to the raw JSON string.
func buildHistoryView(page *familio.HistoryPage) historyPageView {
	view := historyPageView{Data: make([]historyEntryView, 0, len(page.Data)), Pager: page.Pager}
	for _, e := range page.Data {
		var changes any
		if err := json.Unmarshal(e.Record.Changes, &changes); err != nil {
			changes = string(e.Record.Changes)
		}
		view.Data = append(view.Data, historyEntryView{
			Record: historyRecordView{
				ID:              e.Record.ID,
				HappenedAt:      e.Record.HappenedAt,
				Cause:           e.Record.Cause,
				Operation:       e.Record.Operation,
				PersonDataBlock: e.Record.PersonDataBlock,
				Changes:         changes,
			},
			Person: e.Person,
			Author: e.Author,
		})
	}
	return view
}

// runHistoryList lists change-history entries («История изменений», Familio
// Plus), one page per call, with the UI's filters exposed as flags.
func runHistoryList(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("history list")
	var h historyListFlags
	h.register(fs)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) != 0 {
		return errors.New("history list takes no positional arguments")
	}
	filter, err := h.filter()
	if err != nil {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	page, err := c.ListPersonsHistory(ctx, filter)
	if err != nil {
		return err
	}
	return render(g.stdout, buildHistoryView(page))
}

// runHistoryFilters prints the change-history facet vocabularies (authors,
// operations, causes, data types, persons) with their entry counts.
func runHistoryFilters(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("history filters")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) != 0 {
		return errors.New("history filters takes no positional arguments")
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	filters, err := c.GetHistoryFilters(ctx)
	if err != nil {
		return err
	}
	return render(g.stdout, filters)
}

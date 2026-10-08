package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	familio "github.com/dmalch/go-familio"
)

// searchTypes maps the -type values to familio's record types.
var searchTypes = map[string]string{
	"mine":    familio.PersonSearchMine,
	"catalog": familio.PersonSearchCatalog,
	"others":  familio.PersonSearchOtherUsers,
}

// searchOrders maps the -order values to familio's sort fields.
var searchOrders = map[string]string{
	"score":   familio.SearchOrderScore,
	"name":    familio.SearchOrderName,
	"place":   familio.SearchOrderBirthPlace,
	"updated": familio.SearchOrderUpdated,
	"born":    familio.SearchOrderBirth,
	"died":    familio.SearchOrderDeath,
}

// personSearchFlags carries the raw "person search" flag values before they are
// validated into a familio.PersonSearch.
type personSearchFlags struct {
	last       string
	lastExact  bool
	first      string
	firstExact bool
	text       string
	types      multiFlag
	gender     string
	born       string
	died       string
	order      string
	asc        bool
	page       int
	limit      int
}

// register wires the person search flags onto fs.
func (p *personSearchFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&p.last, "last", "", "last name (fuzzy unless -last-exact)")
	fs.BoolVar(&p.lastExact, "last-exact", false, "match the last name exactly")
	fs.StringVar(&p.first, "first", "", "first and middle name (fuzzy unless -first-exact)")
	fs.BoolVar(&p.firstExact, "first-exact", false, "match the first and middle name exactly")
	fs.StringVar(&p.text, "text", "", "free text over the whole name, e.g. \"Мальчиков Иван\"")
	fs.Var(&p.types, "type", "record type: mine|catalog|others (repeatable; default all, mine needs a session)")
	fs.StringVar(&p.gender, "gender", "", "male|female")
	fs.StringVar(&p.born, "born", "", "birth date: YYYY[-MM[-DD]], or a range A..B, A.. or ..B")
	fs.StringVar(&p.died, "died", "", "death date, in the same forms as -born")
	fs.StringVar(&p.order, "order", "score", "sort by score|name|place|updated|born|died")
	fs.BoolVar(&p.asc, "asc", false, "ascending instead of descending order")
	fs.IntVar(&p.page, "page", 1, "1-based page number")
	fs.IntVar(&p.limit, "limit", 20, "results per page")
}

// search validates the raw flags into a familio.PersonSearch.
func (p *personSearchFlags) search() (familio.PersonSearch, error) {
	s := familio.PersonSearch{
		LastName:                p.last,
		LastNameExact:           p.lastExact,
		FirstAndMiddleName:      p.first,
		FirstAndMiddleNameExact: p.firstExact,
		Text:                    p.text,
		Ascending:               p.asc,
		Page:                    p.page,
		ItemsPerPage:            p.limit,
	}
	// familio returns nothing for a search without a name, so say why up front.
	if p.last == "" && p.first == "" && p.text == "" {
		return s, errors.New("give at least one of -last, -first or -text")
	}
	for _, t := range p.types {
		v, ok := searchTypes[t]
		if !ok {
			return s, fmt.Errorf("invalid -type %q: want mine|catalog|others", t)
		}
		s.Types = append(s.Types, v)
	}
	switch p.gender {
	case "", familio.GenderMale, familio.GenderFemale:
		s.Gender = p.gender
	default:
		return s, fmt.Errorf("invalid -gender %q: want male|female", p.gender)
	}
	order, ok := searchOrders[p.order]
	if !ok {
		return s, fmt.Errorf("invalid -order %q: want score|name|place|updated|born|died", p.order)
	}
	s.OrderBy = order

	var err error
	if p.born != "" {
		if s.Born, err = parseDateSpan(p.born); err != nil {
			return s, fmt.Errorf("-born: %w", err)
		}
	}
	if p.died != "" {
		if s.Died, err = parseDateSpan(p.died); err != nil {
			return s, fmt.Errorf("-died: %w", err)
		}
	}
	return s, nil
}

// parseDateSpan parses a -born/-died value: one YYYY[-MM[-DD]] date (see
// parseDate), a closed range A..B, or a range open on one side, A.. or ..B.
func parseDateSpan(v string) (*familio.DateRange, error) {
	if strings.Count(v, "..") > 1 {
		return nil, errors.New("invalid date range " + strconv.Quote(v) + " (want A..B, A.. or ..B)")
	}
	from, till, isRange := strings.Cut(v, "..")
	if !isRange {
		return parseDate(v)
	}
	switch {
	case from == "" && till == "":
		return nil, errors.New("invalid date range " + strconv.Quote(v) + ": no bounds")
	case till == "":
		r, err := parseDate(from)
		if err != nil {
			return nil, err
		}
		r.Range = familio.RangeAfter
		return r, nil
	case from == "":
		r, err := parseDate(till)
		if err != nil {
			return nil, err
		}
		r.Range = familio.RangeBefore
		return r, nil
	}
	r, err := parseDate(from)
	if err != nil {
		return nil, err
	}
	end, err := parseDate(till)
	if err != nil {
		return nil, err
	}
	r.Range = familio.RangeBetween
	r.EndYear, r.EndMonth, r.EndDay = &end.Year, end.Month, end.Day
	return r, nil
}

// runPersonSearch searches familio's persons by name — the account's own, other
// accounts' visible ones and the record catalogs — and prints one page of
// results with its pager.
func runPersonSearch(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("person search")
	var p personSearchFlags
	p.register(fs)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) != 0 {
		return errors.New("person search takes no positional arguments; use -last, -first or -text")
	}
	search, err := p.search()
	if err != nil {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	page, err := c.SearchPersons(ctx, search)
	if err != nil {
		return err
	}
	return render(g.stdout, page)
}

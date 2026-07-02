package main

import (
	"context"
	"errors"
	"net/http"
	"os"

	familio "github.com/dmalch/go-familio"
)

// userAgent identifies the CLI to familio.org.
const userAgent = "go-familio-cli"

// newClient builds a familio.Client from the resolved credentials. The
// settlement commands work with no credentials (public endpoint); the
// authed commands surface familio.ErrNotLoggedIn at call time when no
// usable session was found.
func newClient(g *globalOpts) (*familio.Client, error) {
	cookies, err := resolveCookies(g)
	if err != nil {
		return nil, err
	}
	return familio.NewClient(familio.Options{Cookies: cookies, UserAgent: userAgent})
}

// resolveCookies picks the session cookies using the same precedence as the
// Terraform provider: -cookies/FAMILIO_COOKIES (raw header) > FAMILIO_SESSION
// (bare token) > -browser/FAMILIO_BROWSER (logged-in browser). It returns
// (nil, nil) when nothing is configured so public commands still run.
func resolveCookies(g *globalOpts) ([]*http.Cookie, error) {
	switch {
	case g.cookies != "":
		return familio.CookiesFromHeader(g.cookies), nil
	case os.Getenv("FAMILIO_SESSION") != "":
		return familio.CookieFromSessionToken(os.Getenv("FAMILIO_SESSION")), nil
	case g.browser != "":
		return familio.CookiesFromBrowser(g.browser)
	default:
		return nil, nil
	}
}

// oneArg returns the single positional argument of a command, or an error
// naming what was expected.
func oneArg(args []string, name string) (string, error) {
	if len(args) != 1 || args[0] == "" {
		return "", errors.New("expected exactly one <" + name + "> argument")
	}
	return args[0], nil
}

// parseOneUUID parses a leaf command whose only positional is a single <name>
// argument, honoring the global flags wherever they appear (issue #8). done is
// true when the flag parser already handled -h/-help, in which case the caller
// should return the (nil) error immediately.
func (g *globalOpts) parseOneUUID(cmd, name string, args []string) (uuid string, done bool, err error) {
	fs := g.newFlagSet(cmd)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return "", isHelp(err), ignoreHelp(err)
	}
	uuid, err = oneArg(pos, name)
	return uuid, false, err
}

// ignoreHelp maps flag.ErrHelp to nil (the FlagSet already printed usage) and
// passes any other error through.
func ignoreHelp(err error) error {
	if isHelp(err) {
		return nil
	}
	return err
}

// personView bundles a person's basic record with a normalized relations view
// (parents/spouses/children, derived from the events), convenience birth/death
// years and formatted dates, and the raw life events for a single
// "person get" response. All uuids in relations are full uuids (issue #7).
type personView struct {
	Basic     *familio.BasicRecord `json:"basic"`
	BirthYear *int                 `json:"birthYear,omitempty"`
	DeathYear *int                 `json:"deathYear,omitempty"`
	BirthDate string               `json:"birthDate,omitempty"`
	DeathDate string               `json:"deathDate,omitempty"`
	Relations familio.Relations    `json:"relations"`
	Events    []familio.Event      `json:"events"`
}

// runPersonGet fetches a person's basic record and events by uuid and returns
// them alongside the derived relations, birth/death years, and marriage uuids.
func runPersonGet(ctx context.Context, g *globalOpts, args []string) error {
	uuid, done, err := g.parseOneUUID("person get", "uuid", args)
	if err != nil || done {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	basic, err := c.GetPersonBasic(ctx, uuid)
	if err != nil {
		return err
	}
	events, err := c.GetPersonEvents(ctx, uuid)
	if err != nil {
		return err
	}
	return render(g.stdout, buildPersonView(basic, events, uuid))
}

// buildPersonView assembles the enriched person get response from the raw basic
// record and events.
func buildPersonView(basic *familio.BasicRecord, events []familio.Event, uuid string) personView {
	v := personView{
		Basic:     basic,
		Relations: familio.DeriveRelations(events, uuid),
		Events:    events,
	}
	if year, ok := familio.BirthYear(events, uuid); ok {
		v.BirthYear = &year
	}
	if year, ok := familio.DeathYear(events, uuid); ok {
		v.DeathYear = &year
	}
	if birth := familio.OwnBirthEvent(events, uuid); birth != nil {
		v.BirthDate = birth.Date.Formatted
	}
	if death := familio.OwnDeathEvent(events, uuid); death != nil {
		v.DeathDate = death.Date.Formatted
	}
	return v
}

// runSettlementGet fetches a settlement (place) by uuid.
func runSettlementGet(ctx context.Context, g *globalOpts, args []string) error {
	uuid, done, err := g.parseOneUUID("settlement get", "uuid", args)
	if err != nil || done {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	s, err := c.GetSettlement(ctx, uuid)
	if err != nil {
		return err
	}
	return render(g.stdout, s)
}

// runSettlementPersons lists the persons tied to a settlement. This is a
// public endpoint and needs no credentials.
func runSettlementPersons(ctx context.Context, g *globalOpts, args []string) error {
	uuid, done, err := g.parseOneUUID("settlement persons", "uuid", args)
	if err != nil || done {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	persons, err := c.ListSettlementPersons(ctx, uuid)
	if err != nil {
		return err
	}
	return render(g.stdout, persons)
}

// runSourcesList lists a person's source citations by person uuid.
func runSourcesList(ctx context.Context, g *globalOpts, args []string) error {
	uuid, done, err := g.parseOneUUID("sources list", "person-uuid", args)
	if err != nil || done {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	sources, err := c.GetPersonSources(ctx, uuid)
	if err != nil {
		return err
	}
	return render(g.stdout, sources)
}

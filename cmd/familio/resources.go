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

// personView bundles a person's basic record with their life events for a
// single "person get" response.
type personView struct {
	Basic  *familio.BasicRecord `json:"basic"`
	Events []familio.Event      `json:"events"`
}

// runPersonGet fetches a person's basic record and events by uuid.
func runPersonGet(ctx context.Context, g *globalOpts, args []string) error {
	uuid, err := oneArg(args, "uuid")
	if err != nil {
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
	return render(g.stdout, personView{Basic: basic, Events: events})
}

// runSettlementGet fetches a settlement (place) by uuid.
func runSettlementGet(ctx context.Context, g *globalOpts, args []string) error {
	uuid, err := oneArg(args, "uuid")
	if err != nil {
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
	uuid, err := oneArg(args, "uuid")
	if err != nil {
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
	uuid, err := oneArg(args, "person-uuid")
	if err != nil {
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

package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	familio "github.com/dmalch/go-familio"
)

// catalogPersonUsage names the two ways to point "catalog person" at a record.
const catalogPersonUsage = "expected <catalog-key> <uuid>, or a familio.org/catalogs/<key>/persons/<uuid> link"

// siteURL is familio's web root, for the links the views print. FAMILIO_BASE_URL
// replaces it, as it does the API host.
const siteURL = "https://familio.org/"

// catalogPersonView is what "catalog person" prints: the record with its
// catalog-specific fields under the catalog's titles, as the site's card shows
// them, and its values as text.
type catalogPersonView struct {
	UUID          string             `json:"uuid"`
	URL           string             `json:"url"`
	Catalog       catalogRef         `json:"catalog"`
	Text          string             `json:"text"`
	BirthDate     string             `json:"birthDate,omitempty"`
	DeathDate     string             `json:"deathDate,omitempty"`
	Geography     []string           `json:"geography,omitempty"`
	BirthPlace    string             `json:"birthPlace,omitempty"`
	Fields        []catalogFieldView `json:"fields"`
	LinkedPersons []linkView         `json:"linkedPersons,omitempty"`
	Settlements   []linkView         `json:"settlements,omitempty"`
}

// catalogRef names the catalog a record belongs to.
type catalogRef struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Years string `json:"years,omitempty"`
}

// catalogFieldView is one titled field of a record.
type catalogFieldView struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Value string `json:"value"`
}

// linkView is a person or settlement a record points to.
type linkView struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// runCatalogGet prints a record catalog by its key or uuid: its name, years,
// record count and the fields its records carry.
func runCatalogGet(ctx context.Context, g *globalOpts, args []string) error {
	key, done, err := g.parseOneUUID("catalog get", "catalog-key", args)
	if err != nil || done {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	cat, err := c.GetCatalog(ctx, key)
	if err != nil {
		return err
	}
	return render(g.stdout, cat)
}

// runCatalogPerson prints one catalog record — a "catalogPerson" search result —
// with its fields titled by its catalog. It reads the record and the catalog,
// two requests; the raw record is one "familio api /api/v1/catalogs/<key>/
// excerpts/<uuid>" away.
func runCatalogPerson(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("catalog person")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	key, uuid, err := parseCatalogPersonArgs(pos)
	if err != nil {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	person, err := c.GetCatalogPerson(ctx, key, uuid)
	if err != nil {
		return err
	}
	cat, err := c.GetCatalog(ctx, key)
	if err != nil {
		return err
	}
	site := siteURL
	if g.baseURL != "" {
		site = g.baseURL
	}
	return render(g.stdout, buildCatalogPersonView(person, cat, site))
}

// parseCatalogPersonArgs reads a record's catalog key and uuid from the two
// arguments, or from one familio link to it (https://familio.org/catalogs/
// <key>/persons/<uuid>, or just its path).
func parseCatalogPersonArgs(args []string) (key, uuid string, err error) {
	switch len(args) {
	case 2:
		return args[0], args[1], nil
	case 1:
		u, err := url.Parse(args[0])
		if err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) == 4 && parts[0] == "catalogs" && parts[2] == "persons" && parts[1] != "" && parts[3] != "" {
				return parts[1], parts[3], nil
			}
		}
	}
	return "", "", errors.New(catalogPersonUsage)
}

// buildCatalogPersonView lays a record out under its catalog's titles: the
// fields the site's card shows, in the catalog's order, skipping any without a
// value, with their HTML turned to text.
func buildCatalogPersonView(p *familio.CatalogPerson, cat *familio.Catalog, site string) catalogPersonView {
	v := catalogPersonView{
		UUID:       p.UUID,
		URL:        strings.TrimSuffix(site, "/") + "/catalogs/" + url.PathEscape(p.CatalogKey) + "/persons/" + p.UUID,
		Catalog:    catalogRef{Key: cat.Key, Name: cat.Name, Years: cat.Years},
		Text:       strings.TrimSpace(p.Text),
		Geography:  p.Geography,
		BirthPlace: p.BirthPlace,
		Fields:     []catalogFieldView{},
	}
	if p.BirthDate != nil {
		v.BirthDate = p.BirthDate.Formatted
	}
	if p.DeathDate != nil {
		v.DeathDate = p.DeathDate.Formatted
	}
	for _, f := range cat.Fields {
		if !f.ShowInCard {
			continue
		}
		value := fieldText(p.Record[f.Key])
		if value == "" {
			continue
		}
		v.Fields = append(v.Fields, catalogFieldView{Key: f.Key, Title: f.Title, Value: value})
	}
	for _, lp := range p.LinkedPersons {
		name := strings.Join(strings.Fields(lp.LastName+" "+lp.FirstName+" "+lp.MiddleName), " ")
		v.LinkedPersons = append(v.LinkedPersons, linkView{UUID: lp.UUID, Name: name})
	}
	for _, s := range p.Settlements {
		name := s.Name
		if s.Type != "" {
			name += " (" + s.Type + ")"
		}
		v.Settlements = append(v.Settlements, linkView{UUID: s.UUID, Name: name})
	}
	return v
}

// fieldText renders one record value as text: strings with their HTML turned
// to text, numbers without a fractional part when they have none.
func fieldText(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return htmlToText(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return fmt.Sprint(v)
}

var (
	htmlBreakRe = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlTagRe   = regexp.MustCompile(`<[^>]*>`)
)

// htmlToText turns the markup some catalog values carry into plain text: line
// breaks become newlines, other tags are dropped (a link keeps its text), and
// entities are unescaped.
func htmlToText(s string) string {
	s = htmlBreakRe.ReplaceAllString(s, "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

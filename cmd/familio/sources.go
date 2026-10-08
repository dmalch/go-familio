package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	familio "github.com/dmalch/go-familio"
)

// sourcesAddUsage names the two ways to point "sources add" at a record.
const sourcesAddUsage = "expected <person-uuid> <catalog-key> <record-uuid>, " +
	"or <person-uuid> and the record's familio.org/catalogs/<key>/persons/<uuid> link"

// sourcesRemoveUsage is the argument shape of "sources remove".
const sourcesRemoveUsage = "expected <person-uuid> <record-uuid>, or <person-uuid> and the record's familio link"

// runSourcesAdd cites a catalog record as a person's source — the last step of
// "person search -type catalog" → "catalog person" → here. It reads the record,
// the person and their sources first, refuses a record the person already
// cites, confirms unless -yes, then creates the catalog_person source and, with
// -comment, sets its comment (the same create-then-comment path the Terraform
// provider takes; the create body carries no comment).
func runSourcesAdd(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("sources add")
	comment := fs.String("comment", "", "a comment on the citation")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) < 2 || pos[0] == "" {
		return errors.New(sourcesAddUsage)
	}
	person := pos[0]
	key, record, err := parseCatalogPersonArgs(pos[1:])
	if err != nil {
		return errors.New(sourcesAddUsage)
	}

	c, err := newClient(g)
	if err != nil {
		return err
	}
	rec, err := c.GetCatalogPerson(ctx, key, record)
	if err != nil {
		return err
	}
	display, err := c.GetPersonDisplay(ctx, person)
	if err != nil {
		return err
	}
	existing, err := c.GetPersonSources(ctx, person)
	if err != nil {
		return err
	}
	if familio.FindSourceByID(existing, record) != nil {
		return fmt.Errorf("person %s already cites catalog record %s", person, record)
	}

	target := fmt.Sprintf("«%s» (%s) → «%s» %s", strings.TrimSpace(rec.Text), key, display.DisplayName, person)
	if !*yes && !confirmAction(g, "add", "source(s)", []string{target}) {
		return errors.New("add aborted")
	}

	source, err := c.CreateSource(ctx, person, familio.SourceRef{
		UUID: record, Type: familio.SourceTypeCatalogPerson, CatalogKey: &key,
	})
	if err != nil {
		return err
	}
	if *comment != "" {
		if source, err = c.UpdateSourceComment(ctx, person, source.UUID, *comment); err != nil {
			return fmt.Errorf("source added, but setting its comment failed: %w", err)
		}
	}
	return render(g.stdout, source)
}

// runSourcesRemove removes a source from a person — the undo of "sources add".
// A source is named by the uuid of what it cites, so a catalog record's uuid or
// link names it. A person who does not cite it is an error, with nothing
// deleted; otherwise it confirms unless -yes.
func runSourcesRemove(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("sources remove")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) != 2 || pos[0] == "" || pos[1] == "" {
		return errors.New(sourcesRemoveUsage)
	}
	person, cited := pos[0], pos[1]
	if strings.Contains(cited, "/") {
		if _, cited, err = parseCatalogPersonArgs(pos[1:]); err != nil {
			return errors.New(sourcesRemoveUsage)
		}
	}

	c, err := newClient(g)
	if err != nil {
		return err
	}
	sources, err := c.GetPersonSources(ctx, person)
	if err != nil {
		return err
	}
	source := familio.FindSourceByID(sources, cited)
	if source == nil {
		return fmt.Errorf("person %s does not cite %s", person, cited)
	}

	target := fmt.Sprintf("%s %s on person %s", sourceLabel(source), cited, person)
	if !*yes && !confirmAction(g, "remove", "source(s)", []string{target}) {
		return errors.New("remove aborted")
	}
	if err := c.DeleteSource(ctx, person, cited); err != nil {
		return err
	}
	return render(g.stdout, map[string]any{"person": person, "removed": cited})
}

// sourceLabel names a source for a prompt. A catalog_person source carries the
// record's text in Requisites and its catalog's name in Name; an archive case
// has its own name and requisites. Either way it reads «requisites» (name).
func sourceLabel(s *familio.Source) string {
	requisites := strings.TrimSpace(s.Requisites)
	switch {
	case requisites != "" && s.Name != "":
		return fmt.Sprintf("«%s» (%s)", requisites, s.Name)
	case requisites != "":
		return "«" + requisites + "»"
	case s.Name != "":
		return "«" + s.Name + "»"
	}
	return "source"
}

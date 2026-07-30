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

// tagWriteFlags carries the -name/-color/-description values shared by
// "tags create" and "tags update".
type tagWriteFlags struct {
	name        string
	color       string
	description string
}

// register wires the tag write flags onto fs.
func (t *tagWriteFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&t.name, "name", "", "tag text («Название»); required")
	fs.StringVar(&t.color, "color", "", "palette colour code: "+strings.Join(familio.TagColors, "|")+" (required; see \"tags colors\")")
	fs.StringVar(&t.description, "description", "", "free-text description («Описание»)")
}

// input validates the raw flags into a familio.TagInput, reusing the library's
// own rules so the CLI and the provider reject the same things.
func (t *tagWriteFlags) input() (familio.TagInput, error) {
	in := familio.TagInput{Name: t.name, Color: t.color, Description: t.description}
	if err := in.Validate(); err != nil {
		return in, err
	}
	return in, nil
}

// runTagsList prints the tags the authenticated account owns («Мои метки»).
func runTagsList(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("tags list")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) != 0 {
		return errors.New("tags list takes no positional arguments")
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	tags, err := c.ListTags(ctx)
	if err != nil {
		return err
	}
	return render(g.stdout, tags)
}

// runTagsPerson prints the tags assigned to one person.
func runTagsPerson(ctx context.Context, g *globalOpts, args []string) error {
	uuid, done, err := g.parseOneUUID("tags person", "person-uuid", args)
	if err != nil || done {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	tags, err := c.GetPersonTags(ctx, uuid)
	if err != nil {
		return err
	}
	return render(g.stdout, tags)
}

// runTagsByPersons prints the tags of several persons at once, as a map keyed by
// person uuid. Persons with no tags may be absent from the result.
func runTagsByPersons(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("tags by-persons")
	uuids, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(uuids) == 0 {
		return errors.New("expected at least one <person-uuid> argument")
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	byPerson, err := c.GetTagsByPersons(ctx, uuids)
	if err != nil {
		return err
	}
	return render(g.stdout, byPerson)
}

// tagColor pairs a palette code with the fill the web UI paints it with.
type tagColor struct {
	Color string `json:"color"`
	Hex   string `json:"hex"`
}

// runTagsColors prints the accepted colour codes. familio stores the code, not
// the hex; the hex is here so callers can render tags themselves. This is a
// local lookup, so it makes no request and needs no credentials.
func runTagsColors(_ context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("tags colors")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) != 0 {
		return errors.New("tags colors takes no positional arguments")
	}
	colors := make([]tagColor, 0, len(familio.TagColors))
	for _, code := range familio.TagColors {
		colors = append(colors, tagColor{Color: code, Hex: familio.TagColorHex[code]})
	}
	return render(g.stdout, colors)
}

// runTagsCreate mints a new tag and prints it, including the id the assign and
// update commands take.
func runTagsCreate(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("tags create")
	var w tagWriteFlags
	w.register(fs)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) != 0 {
		return errors.New("tags create takes no positional arguments (pass -name/-color)")
	}
	in, err := w.input()
	if err != nil {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	tag, err := c.CreateTag(ctx, in)
	if err != nil {
		return err
	}
	return render(g.stdout, tag)
}

// runTagsUpdate replaces a tag's name, colour and description. All three are
// sent on every call, so omitting -description clears it.
func runTagsUpdate(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("tags update")
	var w tagWriteFlags
	w.register(fs)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	raw, err := oneArg(pos, "tag-id")
	if err != nil {
		return err
	}
	id, err := parseTagID(raw)
	if err != nil {
		return err
	}
	in, err := w.input()
	if err != nil {
		return err
	}
	c, err := newClient(g)
	if err != nil {
		return err
	}
	tag, err := c.UpdateTag(ctx, id, in)
	if err != nil {
		return err
	}
	return render(g.stdout, tag)
}

// parseTagID parses a tag id. Tags are the one familio resource keyed by a
// small positive integer rather than a uuid, so a uuid-looking argument here is
// a mistake worth naming.
func parseTagID(s string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid <tag-id> %q: want a positive integer (tag ids are numbers, not uuids)", s)
	}
	return id, nil
}

// parseTagIDs parses a list of tag ids, reporting the first bad one.
func parseTagIDs(args []string) ([]int, error) {
	ids := make([]int, 0, len(args))
	for _, arg := range args {
		id, err := parseTagID(arg)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// runTagsDelete deletes tags by id, unassigning them from every person along
// the way. It prompts unless -yes is given.
func runTagsDelete(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("tags delete")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(pos) == 0 {
		return errors.New("expected at least one <tag-id> argument")
	}
	ids, err := parseTagIDs(pos)
	if err != nil {
		return err
	}
	if !*yes && !confirmAction(g, "delete", "tag(s)", pos) {
		return errors.New("delete aborted")
	}

	c, err := newClient(g)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := c.DeleteTag(ctx, id); err != nil {
			return fmt.Errorf("deleting tag %d: %w", id, err)
		}
	}
	return render(g.stdout, map[string]any{"deleted": ids})
}

// runTagsAssign attaches tags to a person and prints the person's refreshed tag
// list, which the endpoint returns.
func runTagsAssign(ctx context.Context, g *globalOpts, args []string) error {
	person, ids, c, err := setUpTagAssignment(g, args, "assign")
	if err != nil || c == nil {
		return err
	}
	tags, err := c.AssignPersonTags(ctx, person, ids)
	if err != nil {
		return err
	}
	return render(g.stdout, map[string]any{"person": person, "tags": tags})
}

// runTagsUnassign detaches tags from a person. The tags themselves survive;
// "tags delete" removes them. The endpoint answers 204 with no body, so the
// refreshed list is read back separately.
func runTagsUnassign(ctx context.Context, g *globalOpts, args []string) error {
	person, ids, c, err := setUpTagAssignment(g, args, "unassign")
	if err != nil || c == nil {
		return err
	}
	if err := c.UnassignPersonTags(ctx, person, ids); err != nil {
		return err
	}
	tags, err := c.GetPersonTags(ctx, person)
	if err != nil {
		return err
	}
	return render(g.stdout, map[string]any{"person": person, "tags": tags})
}

// setUpTagAssignment is the shared front half of assign/unassign: parse
// "<person-uuid> <tag-id>…", confirm the change unless -yes was given, and build
// the client. A nil client with a nil error means the flag parser already
// handled -h/-help and the caller should stop.
func setUpTagAssignment(g *globalOpts, args []string, verb string) (
	person string, ids []int, c *familio.Client, err error) {
	fs := g.newFlagSet("tags " + verb)
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return "", nil, nil, ignoreHelp(err)
	}
	if len(pos) < 2 || pos[0] == "" {
		return "", nil, nil, errors.New("expected <person-uuid> followed by at least one <tag-id>")
	}
	person, rawIDs := pos[0], pos[1:]
	if ids, err = parseTagIDs(rawIDs); err != nil {
		return "", nil, nil, err
	}

	if !*yes && !confirmAction(g, verb, "tag(s) on person "+person, rawIDs) {
		return "", nil, nil, errors.New(verb + " aborted")
	}

	c, err = newClient(g)
	if err != nil {
		return "", nil, nil, err
	}
	return person, ids, c, nil
}

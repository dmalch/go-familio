package main

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"

	familio "github.com/dmalch/go-familio"
)

// twoArgs returns the two positional arguments of a command, or an error naming
// what was expected.
func twoArgs(args []string, first, second string) (string, string, error) {
	if len(args) != 2 || args[0] == "" || args[1] == "" {
		return "", "", errors.New("expected exactly two arguments: <" + first + "> <" + second + ">")
	}
	return args[0], args[1], nil
}

// parseDate parses a non-empty YYYY[-MM[-DD]] date into a DateRange. Callers
// pass an unknown date as an empty string and skip the call (a nil DateRange).
func parseDate(s string) (*familio.DateRange, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "-")
	if len(parts) > 3 {
		return nil, errors.New("invalid date " + strconv.Quote(s) + " (want YYYY[-MM[-DD]])")
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, errors.New("invalid year in date " + strconv.Quote(s))
	}
	r := &familio.DateRange{Year: year}
	if len(parts) >= 2 {
		m, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, errors.New("invalid month in date " + strconv.Quote(s))
		}
		r.Month = &m
	}
	if len(parts) >= 3 {
		d, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, errors.New("invalid day in date " + strconv.Quote(s))
		}
		r.Day = &d
	}
	return r, nil
}

// runMarriageCreate links two existing persons with a wedding event
// (familio's marriage). It prints the created event, whose uuid is the marriage
// (union) uuid.
func runMarriageCreate(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("marriage create")
	var date, comment string
	fs.StringVar(&date, "date", "", "wedding date as YYYY[-MM[-DD]] (optional)")
	fs.StringVar(&comment, "comment", "", "free-text comment (optional)")

	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	partnerA, partnerB, err := twoArgs(pos, "person-a-uuid", "person-b-uuid")
	if err != nil {
		return err
	}
	var dr *familio.DateRange
	if strings.TrimSpace(date) != "" {
		if dr, err = parseDate(date); err != nil {
			return err
		}
	}

	c, err := newClient(g)
	if err != nil {
		return err
	}
	ev, err := c.CreateEvent(ctx, partnerA, familio.WeddingEvent(dr, partnerA, partnerB, comment))
	if err != nil {
		return err
	}
	return render(g.stdout, ev)
}

// runMarriageDelete deletes a wedding event (a marriage). familio addresses an
// event under a participant, so both the participant person uuid and the
// marriage/union uuid are required.
func runMarriageDelete(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("marriage delete")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	personUUID, unionUUID, err := twoArgs(pos, "person-uuid", "union-uuid")
	if err != nil {
		return err
	}

	c, err := newClient(g)
	if err != nil {
		return err
	}
	if err := c.DeleteEvent(ctx, personUUID, unionUUID); err != nil {
		return err
	}
	return render(g.stdout, map[string]string{"deleted": unionUUID})
}

// runPersonSetBiography sets (or appends to) a person's biography. The new text
// comes from -text, or from stdin when -text is not given. -append keeps the
// existing biography and appends the new text after a blank line.
func runPersonSetBiography(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("person set-biography")
	var text string
	var textSet, doAppend bool
	fs.Func("text", "biography text (reads stdin when omitted)", func(s string) error {
		text, textSet = s, true
		return nil
	})
	fs.BoolVar(&doAppend, "append", false, "append to the existing biography instead of replacing it")

	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	uuid, err := oneArg(pos, "uuid")
	if err != nil {
		return err
	}
	if !textSet {
		b, err := io.ReadAll(g.stdin)
		if err != nil {
			return err
		}
		text = string(b)
	}

	c, err := newClient(g)
	if err != nil {
		return err
	}
	bio, err := c.GetPersonBiography(ctx, uuid)
	if err != nil {
		return err
	}
	newText := text
	if doAppend && bio.Text != "" {
		newText = bio.Text + "\n\n" + text
	}
	updated, err := c.UpdatePersonBiography(ctx, uuid, newText, bio.UpdatedAt)
	if err != nil {
		return err
	}
	return render(g.stdout, updated)
}

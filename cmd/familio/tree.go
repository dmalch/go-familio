package main

import (
	"context"
	"errors"

	familio "github.com/dmalch/go-familio"
)

// runTree crawls the persons connected to a root uuid and prints them with
// structured relations (parents/spouses/children) as a JSON array. It replaces
// the ad-hoc BFS crawlers that repeatedly called "person get" and reduced the
// result to {uuid, name, year, parents}.
func runTree(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("tree")
	var up, down, component bool
	var surname string
	var depth int
	fs.BoolVar(&up, "up", false, "follow parents only (ancestors)")
	fs.BoolVar(&down, "down", false, "follow children only (descendants)")
	fs.BoolVar(&component, "component", false, "follow the whole connected component (default)")
	fs.StringVar(&surname, "surname", "", "only expand through people with this last name (bounds in-law branches)")
	fs.IntVar(&depth, "depth", 0, "max BFS depth from the root (0 = unlimited)")

	pos, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	uuid, err := oneArg(pos, "uuid")
	if err != nil {
		return err
	}
	direction, err := treeDirection(up, down, component)
	if err != nil {
		return err
	}

	c, err := newClient(g)
	if err != nil {
		return err
	}
	nodes, err := c.CrawlTree(ctx, uuid, familio.TreeOptions{
		Direction: direction,
		Surname:   surname,
		Depth:     depth,
	})
	if err != nil {
		return err
	}
	return render(g.stdout, nodes)
}

// treeDirection resolves the mutually exclusive -up/-down/-component flags into
// a familio traversal direction, defaulting to the whole connected component.
func treeDirection(up, down, component bool) (string, error) {
	set := 0
	for _, b := range []bool{up, down, component} {
		if b {
			set++
		}
	}
	if set > 1 {
		return "", errors.New("choose at most one of -up, -down, -component")
	}
	switch {
	case up:
		return familio.TreeUp, nil
	case down:
		return familio.TreeDown, nil
	default:
		return familio.TreeComponent, nil
	}
}

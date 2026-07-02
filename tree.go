package familio

import (
	"context"
	"strings"
)

// Tree traversal directions for CrawlTree.
const (
	// TreeUp follows parents only (ancestors).
	TreeUp = "up"
	// TreeDown follows children only (descendants).
	TreeDown = "down"
	// TreeComponent follows parents, spouses, and children (the whole connected
	// component). It is the default.
	TreeComponent = "component"
)

// TreeOptions bounds a CrawlTree walk.
type TreeOptions struct {
	// Direction is TreeUp, TreeDown, or TreeComponent. Empty means TreeComponent.
	Direction string

	// Surname, when set, keeps the crawl from expanding through people whose last
	// name (or maiden last name) does not match — the way to avoid pulling living
	// in-law branches into an otherwise on-surname component. Non-matching people
	// are still emitted as nodes (they are referenced), they are just not
	// expanded. The root is always expanded. Matching is case-insensitive.
	Surname string

	// Depth caps the BFS distance from the root (root = 0). Zero means unlimited.
	Depth int
}

// TreeNode is one person in a crawled tree with their normalized relations.
// UUID and every relation uuid are full uuids. Year is the birth year (omitted
// when unknown).
type TreeNode struct {
	UUID     string      `json:"uuid"`
	Name     string      `json:"name,omitempty"`
	Year     int         `json:"year,omitempty"`
	Parents  []PersonRef `json:"parents"`
	Spouses  []Spouse    `json:"spouses"`
	Children []PersonRef `json:"children"`
}

// crawlItem is a queued BFS entry.
type crawlItem struct {
	uuid  string
	depth int
}

// CrawlTree walks the connected persons around rootUUID and returns them with
// structured relations, in breadth-first discovery order. It replaces hand-
// written BFS crawlers that called person-get repeatedly and reduced the result
// to {uuid, name, year, parents}. Direction, Surname, and Depth bound the walk
// (see TreeOptions). Every node fetches the person's events once; relations,
// name, and birth year are derived from them.
func (c *Client) CrawlTree(ctx context.Context, rootUUID string, opts TreeOptions) ([]TreeNode, error) {
	direction := opts.Direction
	if direction == "" {
		direction = TreeComponent
	}

	visited := map[string]bool{}
	queue := []crawlItem{{uuid: rootUUID, depth: 0}}
	var nodes []TreeNode

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if visited[item.uuid] {
			continue
		}
		visited[item.uuid] = true

		node, lastName, err := c.fetchTreeNode(ctx, item.uuid, opts.Surname != "")
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)

		if opts.Depth > 0 && item.depth >= opts.Depth {
			continue
		}
		// Only expand through people on the requested surname (the root always
		// expands, so a surname-bounded crawl still leaves its starting person).
		if opts.Surname != "" && item.uuid != rootUUID && !surnameMatches(lastName, opts.Surname) {
			continue
		}

		for _, uuid := range neighbors(node, direction) {
			if !visited[uuid] {
				queue = append(queue, crawlItem{uuid: uuid, depth: item.depth + 1})
			}
		}
	}

	return nodes, nil
}

// fetchTreeNode reads one person's events and reduces them to a TreeNode. When
// needSurname is set it also reads the basic record to return the person's last
// name (and its maiden variant) for the surname filter.
func (c *Client) fetchTreeNode(ctx context.Context, uuid string, needSurname bool) (TreeNode, []string, error) {
	events, err := c.GetPersonEvents(ctx, uuid)
	if err != nil {
		return TreeNode{}, nil, err
	}

	rel := DeriveRelations(events, uuid)
	node := TreeNode{
		UUID:     uuid,
		Name:     displayNames(events)[uuid],
		Parents:  rel.Parents,
		Spouses:  rel.Spouses,
		Children: rel.Children,
	}
	if year, ok := BirthYear(events, uuid); ok {
		node.Year = year
	}

	var lastNames []string
	if needSurname {
		basic, err := c.GetPersonBasic(ctx, uuid)
		if err != nil {
			return TreeNode{}, nil, err
		}
		lastNames = []string{basic.LastName, basic.BirthLastName}
		if node.Name == "" {
			node.Name = strings.TrimSpace(basic.FirstName + " " + basic.LastName)
		}
	}

	return node, lastNames, nil
}

// neighbors returns the uuids to expand into from node for the given direction.
func neighbors(node TreeNode, direction string) []string {
	var out []string
	if direction == TreeUp || direction == TreeComponent {
		for _, p := range node.Parents {
			out = append(out, p.UUID)
		}
	}
	if direction == TreeComponent {
		for _, s := range node.Spouses {
			out = append(out, s.UUID)
		}
	}
	if direction == TreeDown || direction == TreeComponent {
		for _, ch := range node.Children {
			out = append(out, ch.UUID)
		}
	}
	return out
}

// surnameMatches reports whether any of a person's last-name variants equals
// want, case-insensitively (ignoring surrounding whitespace).
func surnameMatches(lastNames []string, want string) bool {
	want = strings.TrimSpace(strings.ToLower(want))
	for _, n := range lastNames {
		if strings.TrimSpace(strings.ToLower(n)) == want {
			return true
		}
	}
	return false
}

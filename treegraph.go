package familio

import (
	"context"
	"net/http"
	"strings"
)

// Node roles in a TreeGraph, relative to the account's central person.
const (
	// TreeRoleCentralPerson is the person the graph is centred on — the account
	// holder's own person. Exactly one node has it, at Layer 0.
	TreeRoleCentralPerson = "central_person"
	// TreeRoleParent is an ancestor of the central person.
	TreeRoleParent = "parent"
	// TreeRoleChild is a descendant of the central person.
	TreeRoleChild = "child"
	// TreeRoleSpouse is a partner of the central person.
	TreeRoleSpouse = "spouse"
)

// TreeGraph is the account's tree as familio's editor canvas loads it: one node
// per placement, each carrying both its position in the layout and a summary of
// the person. It is the cheap way to see a tree — one request, where CrawlTree
// spends one per person — and it already includes names, pre-formatted dates,
// localities and photos.
//
// It is a *window*, not necessarily the whole tree: the graph is centred on the
// account's own person, and a node whose Params.HasMore is true has further
// relatives this response does not include. Use CrawlTree when you need
// guaranteed reach, or structured (parseable) dates and event uuids.
type TreeGraph struct {
	Nodes []TreeGraphNode `json:"nodes"`
	Edges TreeGraphEdges  `json:"edges"`
}

// TreeGraphEdges is the graph's explicit edge list, as the layout engine consumes
// it. It duplicates what the per-node Parents lists say.
type TreeGraphEdges struct {
	// LayoutBasis links parent (Source) to child (Target), by node id.
	LayoutBasis []TreeGraphLink `json:"layoutBasis"`
}

// TreeGraphLink is one parent → child edge between node ids.
type TreeGraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// TreeGraphNode is one person's placement in the graph.
//
// NodeID is *not* a person uuid: it is the person uuid plus a placement suffix
// (`<uuid>.1`), because one person can appear at more than one spot in a layout.
// Person.PersonID carries the uuid, and PersonUUID derives it from either.
type TreeGraphNode struct {
	NodeID string          `json:"nodeId"`
	Params TreeGraphParams `json:"nodeParams"`
	Person TreeGraphPerson `json:"personData"`
}

// PersonUUID returns the node's person uuid, taken from personData when present
// and otherwise derived from the node id.
func (n *TreeGraphNode) PersonUUID() string {
	if n.Person.PersonID != "" {
		return n.Person.PersonID
	}
	return PersonUUIDFromNodeID(n.NodeID)
}

// PersonUUIDFromNodeID strips a graph node id's placement suffix, turning
// "<uuid>.1" into the person uuid every other endpoint speaks. The Parents and
// Partners edge lists carry node ids, so resolving one to a person goes through
// here. A value with no suffix is returned unchanged.
func PersonUUIDFromNodeID(nodeID string) string {
	if dot := strings.LastIndexByte(nodeID, '.'); dot > 0 {
		return nodeID[:dot]
	}
	return nodeID
}

// TreeGraphParams is a node's position in the graph: its kinship role relative to
// the central person, its edges, and the layout hints familio's canvas uses.
//
// The two edge lists are asymmetric — Parents are objects carrying a sex hint,
// Partners are bare node ids — and there is no children list: a child is the
// inverse of a Parents edge (or a LayoutBasis link).
type TreeGraphParams struct {
	// Role is TreeRoleCentralPerson, TreeRoleParent, TreeRoleChild or
	// TreeRoleSpouse.
	Role string `json:"role"`

	// RoleName / RoleShortName / RoleNameAccusative are familio's Russian kinship
	// labels for this node relative to the central person («Прадедушка»,
	// «прадедушку»). Empty on the central person itself.
	RoleName           string `json:"roleName"`
	RoleShortName      string `json:"roleShortName"`
	RoleNameAccusative string `json:"roleNameAccusative"`

	// Parents are this node's 0–2 parent edges, by node id, with the sex hint the
	// layout places them by.
	Parents []TreeGraphEdge `json:"parents"`

	// Partners are the node ids this node is partnered with (bare ids, unlike
	// Parents).
	Partners []string `json:"partners"`

	// Layer is the generational distance from the central person (0 = the central
	// person, higher = further back).
	Layer int `json:"layer"`

	// IsPlaceholder marks an empty slot the canvas draws rather than a real
	// person.
	IsPlaceholder bool `json:"isPlaceholder"`

	// HasMore marks a node with further relatives that this response does not
	// include — the signal that the graph is a window on the tree.
	HasMore bool `json:"hasMore"`

	// IsRecursionFound marks a node the layout reached twice through a cycle.
	IsRecursionFound bool `json:"isRecursionFound"`
}

// TreeGraphEdge points at another node, with the sex familio uses to place it.
type TreeGraphEdge struct {
	Sex    string `json:"sex"`
	NodeID string `json:"nodeId"`
}

// TreeGraphPerson is the person summary the graph embeds — enough to render a
// tree card without a per-person request.
//
// The dates are familio's **display** strings, not parseable values: they are
// day-first Russian, may carry a qualifier («После 29.04.1926»), and mark
// Julian/old-style dates with «ст.» (`29.11.1890 ст.`). For structured dates read
// the person's events and use RangeFromEventDate.
type TreeGraphPerson struct {
	// PersonID is the person uuid — what GetPersonBasic and friends take.
	PersonID string `json:"personId"`

	Sex        string `json:"sex"`
	LastName   string `json:"lastName"`
	FirstName  string `json:"firstName"`
	Patronymic string `json:"patronymic"`
	// BirthFirstName / BirthLastName are the names at birth (maiden name).
	BirthFirstName string `json:"birthFirstName"`
	BirthLastName  string `json:"birthLastName"`
	// Initials is familio's pre-rendered two-letter monogram («ДМ»).
	Initials string `json:"initials"`

	// Photo is a site-relative thumbnail path, empty when there is none.
	Photo string `json:"photo"`
	// Locality is the settlement label familio shows on the card.
	Locality string `json:"locality"`

	// DateBirth / DateDeath are display strings (see the type comment); Age is a
	// rendered span («41 год»), empty when it cannot be computed.
	DateBirth string `json:"dateBirth"`
	DateDeath string `json:"dateDeath"`
	Age       string `json:"age"`
	// HasDeathEvent distinguishes "no death recorded" from "death with no date".
	HasDeathEvent bool `json:"hasDeathEvent"`

	// IsPrivate is the person's privacy setting; IsMe marks the account holder's
	// own person; IsMine marks a person this account owns; IsUserPerson marks a
	// profile created by a user rather than sourced from a catalog.
	IsPrivate    bool `json:"isPrivate"`
	IsMe         bool `json:"isMe"`
	IsMine       bool `json:"isMine"`
	IsUserPerson bool `json:"isUserPerson"`

	// Owner is the account uuid that owns the person.
	Owner string `json:"owner"`

	// The per-block last-modified stamps familio tracks. These are *not* the
	// X-Base-Version tokens for a write — read those from the resource itself.
	BasicUpdatedAt     string `json:"basicUpdatedAt"`
	PhotoUpdatedAt     string `json:"photoUpdatedAt"`
	BiographyUpdatedAt string `json:"biographyUpdatedAt"`
}

// GetTreeGraph reads the account's tree in one request (GET /api/v2/tree),
// centred on the account holder's own person. See TreeGraph for what it does and
// does not cover; CrawlTree is the rooted, bounded alternative.
func (c *Client) GetTreeGraph(ctx context.Context) (*TreeGraph, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodGet, "tree", nil, nil)
	if err != nil {
		return nil, err
	}
	var graph TreeGraph
	if err := c.do(req, &graph); err != nil {
		return nil, err
	}
	return &graph, nil
}

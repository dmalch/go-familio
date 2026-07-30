package familio

import (
	"context"
	"net/http"
)

// TreeGraph is the account's whole tree as familio's editor loads it: one node
// per person, with the kinship edges denormalized onto each node. It is the
// cheap way to learn a tree's shape — one request, where CrawlTree spends one
// per person — but it carries no names, dates, or places. Use CrawlTree when you
// need those.
type TreeGraph struct {
	Nodes []TreeGraphNode `json:"nodes"`
}

// TreeGraphNode is one person in the graph. NodeID is their person uuid, so it
// feeds GetPersonBasic / GetPersonEvents directly.
type TreeGraphNode struct {
	NodeID string          `json:"nodeId"`
	Params TreeGraphParams `json:"nodeParams"`
}

// TreeGraphParams holds a node's position in the graph: its role in the layout
// and its edges to parents and partners. There is no children list — a child is
// the inverse of a Parents edge, so walk the nodes to invert it.
type TreeGraphParams struct {
	Role     string          `json:"role"`
	Parents  []TreeGraphEdge `json:"parents"`
	Partners []TreeGraphEdge `json:"partners"`
}

// TreeGraphEdge points at another node in the graph. Sex is GenderMale or
// GenderFemale — the layout hint familio uses to place the person, and enough to
// tell a father edge from a mother edge without reading the person.
type TreeGraphEdge struct {
	Sex    string `json:"sex"`
	NodeID string `json:"nodeId"`
}

// GetTreeGraph reads the authenticated account's tree in one request
// (GET /api/v2/tree). Unlike CrawlTree it is not rooted or bounded: it returns
// every person in the account's tree.
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

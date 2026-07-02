package familio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// treeFixtureEvents maps each person uuid to the JSON its /events endpoint
// returns. The graph: r (Иванов) is child of f (Иванов), married to w
// (Петрова), and parent of c (Иванов); w is child of wp (Петров).
var treeFixtureEvents = map[string]string{
	"r": `[
	  {"uuid":"b_r","type":"birth","date":{"first":{"year":1826},"formatted":"1826"},
	   "participants":[{"personUuid":"r","role":"child","displayName":"Иван Иванов"},
	                   {"personUuid":"f","role":"parent","displayName":"Пётр Иванов"}]},
	  {"uuid":"w_rw","type":"wedding",
	   "participants":[{"personUuid":"r","role":"spouse","displayName":"Иван Иванов"},
	                   {"personUuid":"w","role":"spouse","displayName":"Анна Петрова"}]},
	  {"uuid":"b_c","type":"birth",
	   "participants":[{"personUuid":"c","role":"child","displayName":"Сын Иванов"},
	                   {"personUuid":"r","role":"parent","displayName":"Иван Иванов"}]}
	]`,
	"f": `[
	  {"uuid":"b_r","type":"birth",
	   "participants":[{"personUuid":"r","role":"child","displayName":"Иван Иванов"},
	                   {"personUuid":"f","role":"parent","displayName":"Пётр Иванов"}]},
	  {"uuid":"b_f","type":"birth","date":{"first":{"year":1800}},
	   "participants":[{"personUuid":"f","role":"child","displayName":"Пётр Иванов"}]}
	]`,
	"w": `[
	  {"uuid":"w_rw","type":"wedding",
	   "participants":[{"personUuid":"r","role":"spouse","displayName":"Иван Иванов"},
	                   {"personUuid":"w","role":"spouse","displayName":"Анна Петрова"}]},
	  {"uuid":"b_w","type":"birth",
	   "participants":[{"personUuid":"w","role":"child","displayName":"Анна Петрова"},
	                   {"personUuid":"wp","role":"parent","displayName":"Отец Петров"}]}
	]`,
	"c": `[
	  {"uuid":"b_c","type":"birth",
	   "participants":[{"personUuid":"c","role":"child","displayName":"Сын Иванов"},
	                   {"personUuid":"r","role":"parent","displayName":"Иван Иванов"}]}
	]`,
	"wp": `[
	  {"uuid":"b_w","type":"birth",
	   "participants":[{"personUuid":"w","role":"child","displayName":"Анна Петрова"},
	                   {"personUuid":"wp","role":"parent","displayName":"Отец Петров"}]}
	]`,
}

var treeFixtureLastName = map[string]string{
	"r": "Иванов", "f": "Иванов", "c": "Иванов", "w": "Петрова", "wp": "Петров",
}

// treeFixtureServer serves the bearer token at "/" and the events/basic
// sub-resources for the fixture graph.
func treeFixtureServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, `<script id="__NEXT_DATA__">{"props":{"token":"eyJ.eyJ.sig"}}</script>`)
			return
		}
		// /api/v2/persons/<uuid>/events or /basic
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v2/persons/"), "/")
		uuid, sub := parts[0], parts[1]
		switch sub {
		case "events":
			_, _ = io.WriteString(w, treeFixtureEvents[uuid])
		case "basic":
			_, _ = io.WriteString(w, `{"uuid":"`+uuid+`","lastName":"`+treeFixtureLastName[uuid]+`","firstName":"Имя"}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func newTreeClient(srv *httptest.Server) *Client {
	c, _ := NewClient(Options{BaseURL: srv.URL + "/", Cookies: CookiesFromHeader("t=secret"), RateLimit: 1000})
	return c
}

func treeUUIDs(nodes []TreeNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.UUID
	}
	return out
}

func TestCrawlTree_Component(t *testing.T) {
	RegisterTestingT(t)
	srv := treeFixtureServer()
	defer srv.Close()

	nodes, err := newTreeClient(srv).CrawlTree(context.Background(), "r", TreeOptions{Direction: TreeComponent})
	Expect(err).ToNot(HaveOccurred())
	Expect(treeUUIDs(nodes)).To(ConsistOf("r", "f", "w", "c", "wp"))

	// The root carries derived relations, a name, and a birth year.
	var root TreeNode
	for _, n := range nodes {
		if n.UUID == "r" {
			root = n
		}
	}
	Expect(root.Name).To(Equal("Иван Иванов"))
	Expect(root.Year).To(Equal(1826))
	Expect(root.Parents).To(ConsistOf(PersonRef{UUID: "f", Name: "Пётр Иванов"}))
	Expect(root.Children).To(ConsistOf(PersonRef{UUID: "c", Name: "Сын Иванов"}))
	Expect(root.Spouses).To(ConsistOf(Spouse{UUID: "w", Name: "Анна Петрова", MarriageUUID: "w_rw"}))
}

// TestCrawlTree_SurnameBoundsInLaws proves a surname filter emits the spouse
// (Петрова) but does not expand through her into her own parents.
func TestCrawlTree_SurnameBoundsInLaws(t *testing.T) {
	RegisterTestingT(t)
	srv := treeFixtureServer()
	defer srv.Close()

	nodes, err := newTreeClient(srv).CrawlTree(context.Background(), "r", TreeOptions{
		Direction: TreeComponent, Surname: "Иванов",
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(treeUUIDs(nodes)).To(ConsistOf("r", "f", "w", "c"))
	Expect(treeUUIDs(nodes)).ToNot(ContainElement("wp"))
}

func TestCrawlTree_Up(t *testing.T) {
	RegisterTestingT(t)
	srv := treeFixtureServer()
	defer srv.Close()

	nodes, err := newTreeClient(srv).CrawlTree(context.Background(), "r", TreeOptions{Direction: TreeUp})
	Expect(err).ToNot(HaveOccurred())
	Expect(treeUUIDs(nodes)).To(ConsistOf("r", "f"))
}

func TestCrawlTree_Down(t *testing.T) {
	RegisterTestingT(t)
	srv := treeFixtureServer()
	defer srv.Close()

	nodes, err := newTreeClient(srv).CrawlTree(context.Background(), "r", TreeOptions{Direction: TreeDown})
	Expect(err).ToNot(HaveOccurred())
	Expect(treeUUIDs(nodes)).To(ConsistOf("r", "c"))
}

func TestCrawlTree_DepthOne(t *testing.T) {
	RegisterTestingT(t)
	srv := treeFixtureServer()
	defer srv.Close()

	// Depth 1 from r reaches immediate relations (f, w, c) but not wp (2 hops).
	nodes, err := newTreeClient(srv).CrawlTree(context.Background(), "r", TreeOptions{
		Direction: TreeComponent, Depth: 1,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(treeUUIDs(nodes)).To(ConsistOf("r", "f", "w", "c"))
}

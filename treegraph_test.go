package familio

import (
	"context"
	"io"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
)

// treeGraphFixture is the shape GET /api/v2/tree returns: one node per person in
// the account's tree, each naming its parents and partners by nodeId (see API.md
// "Persons — authed read"). Node ids are person uuids.
const treeGraphFixture = `{"nodes": [
  {"nodeId": "a01a51d0-b736-493b-8b3f-8096ac20a07c",
   "nodeParams": {"role": "root", "parents": [], "partners": []}},
  {"nodeId": "15acfcd4-90b2-4633-a50e-04f1bc57d850",
   "nodeParams": {"role": "child",
     "parents": [{"sex": "male", "nodeId": "a01a51d0-b736-493b-8b3f-8096ac20a07c"},
                 {"sex": "female", "nodeId": "b2b0c8f1-1111-4222-8333-444455556666"}],
     "partners": [{"sex": "female", "nodeId": "c3c1d9e2-7777-4888-9999-aaaabbbbcccc"}]}}
]}`

// TestGetTreeGraph locks the one-request tree read and its edge decode.
func TestGetTreeGraph(t *testing.T) {
	RegisterTestingT(t)
	srv := authedTestServer(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.Method).To(Equal(http.MethodGet))
		Expect(r.URL.Path).To(Equal("/api/v2/tree"))
		Expect(r.Header.Get("Authorization")).To(HavePrefix("Bearer eyJ"))
		_, _ = io.WriteString(w, treeGraphFixture)
	})
	defer srv.Close()

	graph, err := newTestClient(srv).GetTreeGraph(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(graph.Nodes).To(HaveLen(2))

	root := graph.Nodes[0]
	Expect(root.NodeID).To(Equal("a01a51d0-b736-493b-8b3f-8096ac20a07c"))
	Expect(root.Params.Role).To(Equal("root"))
	Expect(root.Params.Parents).To(BeEmpty())
	Expect(root.Params.Partners).To(BeEmpty())

	child := graph.Nodes[1]
	Expect(child.Params.Parents).To(HaveLen(2))
	Expect(child.Params.Parents[0].NodeID).To(Equal("a01a51d0-b736-493b-8b3f-8096ac20a07c"))
	Expect(child.Params.Parents[0].Sex).To(Equal(GenderMale))
	Expect(child.Params.Parents[1].Sex).To(Equal(GenderFemale))
	Expect(child.Params.Partners).To(HaveLen(1))
	Expect(child.Params.Partners[0].NodeID).To(Equal("c3c1d9e2-7777-4888-9999-aaaabbbbcccc"))
}

package familio

import (
	"context"
	"io"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
)

// treeGraphFixture is a trimmed live response of GET /api/v2/tree: the central
// person, one parent, and the partner edge between them. Names and dates are
// real-shaped — familio pre-formats them in Russian, old-style dates included
// («ст.»), and the ids are node ids (personUuid + a placement suffix), not bare
// person uuids. See API.md "Tree graph".
const treeGraphFixture = `{
  "nodes": [
    {"nodeId": "c6a3ecfe-1304-4303-a1f7-eb33f9e1e3e0.1",
     "nodeParams": {"role": "central_person", "roleShortName": "", "roleName": "",
       "roleNameAccusative": "",
       "parents": [{"sex": "male", "nodeId": "782adc64-6e57-4c0d-9e2b-578f5726762c.1"}],
       "partners": ["9d4c544b-79df-41fc-b128-5f35ef6fb8b2.1"],
       "layer": 0, "isPlaceholder": false, "hasMore": false, "isRecursionFound": false},
     "personData": {"sex": "male", "personId": "c6a3ecfe-1304-4303-a1f7-eb33f9e1e3e0",
       "lastName": "Мальчиков", "firstName": "Дмитрий", "patronymic": "Викторович",
       "photo": "", "locality": "", "dateBirth": "09.05.1985", "dateDeath": "",
       "birthFirstName": "", "birthLastName": "", "isPrivate": true, "isMe": true,
       "isMine": true, "isUserPerson": true, "initials": "ДМ", "hasDeathEvent": false,
       "age": "41 год", "owner": "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0",
       "basicUpdatedAt": "2024-07-18T12:19:58+00:00"}},
    {"nodeId": "782adc64-6e57-4c0d-9e2b-578f5726762c.1",
     "nodeParams": {"role": "parent", "roleShortName": "Прадедушка", "roleName": "Прадедушка",
       "roleNameAccusative": "прадедушку",
       "parents": [], "partners": [], "layer": 3, "isPlaceholder": false,
       "hasMore": true, "isRecursionFound": false},
     "personData": {"sex": "male", "personId": "782adc64-6e57-4c0d-9e2b-578f5726762c",
       "lastName": "Мальчиков", "firstName": "Николай", "patronymic": "Васильевич",
       "photo": "/images/user_files/…/977e6cab.thumb-400x400.jpg", "locality": "Кириллово",
       "dateBirth": "29.11.1890 ст.", "dateDeath": "После 29.04.1926",
       "birthFirstName": "", "birthLastName": "", "isPrivate": false, "isMe": false,
       "isMine": true, "isUserPerson": false, "initials": "НМ", "hasDeathEvent": true,
       "age": "", "owner": "894dc7d5-65f3-4c60-ad4e-3084f0bc26e0"}}
  ],
  "edges": {"layoutBasis": [
    {"source": "782adc64-6e57-4c0d-9e2b-578f5726762c.1",
     "target": "c6a3ecfe-1304-4303-a1f7-eb33f9e1e3e0.1"}]},
  "theme": [],
  "recursiveNodes": []
}`

// TestGetTreeGraph locks the one-request tree read: the node/person split, the
// asymmetry between the two edge lists (parents are objects, partners are bare
// node ids), and the paging signal.
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

	central := graph.Nodes[0]
	Expect(central.NodeID).To(Equal("c6a3ecfe-1304-4303-a1f7-eb33f9e1e3e0.1"))
	Expect(central.Params.Role).To(Equal(TreeRoleCentralPerson))
	Expect(central.Params.Layer).To(BeZero())
	Expect(central.Params.HasMore).To(BeFalse())

	// The person uuid is not the node id: a person can be placed more than once.
	Expect(central.Person.PersonID).To(Equal("c6a3ecfe-1304-4303-a1f7-eb33f9e1e3e0"))
	Expect(central.PersonUUID()).To(Equal(central.Person.PersonID))
	Expect(central.Person.FirstName).To(Equal("Дмитрий"))
	Expect(central.Person.Patronymic).To(Equal("Викторович"))
	Expect(central.Person.DateBirth).To(Equal("09.05.1985"))
	Expect(central.Person.Age).To(Equal("41 год"))
	Expect(central.Person.IsMe).To(BeTrue())
	Expect(central.Person.IsPrivate).To(BeTrue())

	// parents are objects carrying the layout sex hint…
	Expect(central.Params.Parents).To(HaveLen(1))
	Expect(central.Params.Parents[0].Sex).To(Equal(GenderMale))
	Expect(central.Params.Parents[0].NodeID).To(Equal("782adc64-6e57-4c0d-9e2b-578f5726762c.1"))
	// …while partners are bare node ids.
	Expect(central.Params.Partners).To(ConsistOf("9d4c544b-79df-41fc-b128-5f35ef6fb8b2.1"))

	parent := graph.Nodes[1]
	Expect(parent.Params.Role).To(Equal(TreeRoleParent))
	Expect(parent.Params.RoleName).To(Equal("Прадедушка"))
	Expect(parent.Params.Layer).To(Equal(3))
	Expect(parent.Params.HasMore).To(BeTrue(), "hasMore marks relatives the response omits")
	Expect(parent.Person.DateBirth).To(Equal("29.11.1890 ст."), "familio pre-formats dates, old style included")
	Expect(parent.Person.DateDeath).To(Equal("После 29.04.1926"))
	Expect(parent.Person.Locality).To(Equal("Кириллово"))
	Expect(parent.Person.HasDeathEvent).To(BeTrue())

	Expect(graph.Edges.LayoutBasis).To(HaveLen(1))
	Expect(graph.Edges.LayoutBasis[0].Source).To(Equal(parent.NodeID), "source is the parent")
	Expect(graph.Edges.LayoutBasis[0].Target).To(Equal(central.NodeID), "target is the child")
}

// TestPersonUUIDFromNodeID covers resolving an edge's node id back to a person:
// the parents/partners lists speak node ids, and every lookup elsewhere in the
// API wants the uuid.
func TestPersonUUIDFromNodeID(t *testing.T) {
	RegisterTestingT(t)
	Expect(PersonUUIDFromNodeID("c6a3ecfe-1304-4303-a1f7-eb33f9e1e3e0.1")).
		To(Equal("c6a3ecfe-1304-4303-a1f7-eb33f9e1e3e0"))
	// Already a bare uuid, or empty: returned unchanged.
	Expect(PersonUUIDFromNodeID("c6a3ecfe-1304-4303-a1f7-eb33f9e1e3e0")).
		To(Equal("c6a3ecfe-1304-4303-a1f7-eb33f9e1e3e0"))
	Expect(PersonUUIDFromNodeID("")).To(BeEmpty())
}

// TestTreeGraphNodePersonUUIDFallsBackToTheNodeID keeps the helper useful on a
// node whose personData is absent (a placeholder slot in the layout).
func TestTreeGraphNodePersonUUIDFallsBackToTheNodeID(t *testing.T) {
	RegisterTestingT(t)
	node := TreeGraphNode{NodeID: "782adc64-6e57-4c0d-9e2b-578f5726762c.0"}
	Expect(node.PersonUUID()).To(Equal("782adc64-6e57-4c0d-9e2b-578f5726762c"))
}

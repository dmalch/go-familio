package familio

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// TestGetProfileLive hits the real /profile and /tree endpoints to prove the wire
// types decode against production data. Read-only. Skipped unless
// FAMILIO_NETWORK_TEST=1, so CI never runs it.
func TestGetProfileLive(t *testing.T) {
	if os.Getenv("FAMILIO_NETWORK_TEST") != "1" {
		t.Skip("set FAMILIO_NETWORK_TEST=1 to run the live familio.org decode test")
	}
	RegisterTestingT(t)

	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	profile, err := client.GetProfile(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(profile.User.UUID).ToNot(BeEmpty())
	Expect(profile.User.Email).ToNot(BeEmpty())
	Expect(profile.Details.DisplayName).ToNot(BeEmpty())

	// The profile's uuid must be the same account the JWT claim names.
	accountUUID, err := client.AccountUUID(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(profile.User.UUID).To(Equal(accountUUID),
		"the /profile uuid and the JWT uuid claim should name the same account")

	t.Logf("account %s <%s> — %s", profile.User.UUID, profile.User.Email, profile.Details.DisplayName)
}

// TestGetTreeGraphLive proves the one-request tree read decodes, and that its
// node ids really are person uuids resolvable through the persons endpoint.
func TestGetTreeGraphLive(t *testing.T) {
	if os.Getenv("FAMILIO_NETWORK_TEST") != "1" {
		t.Skip("set FAMILIO_NETWORK_TEST=1 to run the live familio.org decode test")
	}
	RegisterTestingT(t)

	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	graph, err := client.GetTreeGraph(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(graph.Nodes).ToNot(BeEmpty(), "the account should have at least one person")

	byNodeID := make(map[string]TreeGraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		byNodeID[node.NodeID] = node
	}

	var central, hasMore, edges int
	for _, node := range graph.Nodes {
		Expect(node.NodeID).ToNot(BeEmpty())
		Expect(node.PersonUUID()).ToNot(BeEmpty())
		Expect(node.NodeID).To(HavePrefix(node.PersonUUID()),
			"a node id should be its person uuid plus a placement suffix")
		Expect([]string{TreeRoleCentralPerson, TreeRoleParent, TreeRoleChild, TreeRoleSpouse}).
			To(ContainElement(node.Params.Role), "unexpected role %q", node.Params.Role)
		if node.Params.Role == TreeRoleCentralPerson {
			central++
			Expect(node.Params.Layer).To(BeZero(), "the central person sits at layer 0")
		}
		if node.Params.HasMore {
			hasMore++
		}

		for _, parent := range node.Params.Parents {
			Expect(parent.NodeID).ToNot(BeEmpty())
			Expect([]string{GenderMale, GenderFemale}).To(ContainElement(parent.Sex),
				"unexpected sex %q on a parent edge of %s", parent.Sex, node.NodeID)
			edges++
		}
		// Partners are bare node ids, unlike parents.
		for _, partner := range node.Params.Partners {
			Expect(partner).ToNot(BeEmpty())
			edges++
		}
	}
	Expect(central).To(Equal(1), "exactly one node is the central person")
	t.Logf("decoded %d node(s), %d edge(s), %d node(s) with more relatives beyond the window",
		len(graph.Nodes), edges, hasMore)

	// Every edge must point at a node the response actually contains.
	for _, link := range graph.Edges.LayoutBasis {
		Expect(byNodeID).To(HaveKey(link.Source))
		Expect(byNodeID).To(HaveKey(link.Target))
	}

	// The embedded person summary must line up with the person endpoint.
	first := graph.Nodes[0]
	Expect(first.Person.FirstName).ToNot(BeEmpty())
	basic, err := client.GetPersonBasic(ctx, first.PersonUUID())
	Expect(err).ToNot(HaveOccurred())
	Expect(basic.UUID).To(Equal(first.PersonUUID()))
	Expect(basic.FirstName).To(Equal(first.Person.FirstName))
	Expect(basic.LastName).To(Equal(first.Person.LastName))
	Expect(basic.MiddleName).To(Equal(first.Person.Patronymic),
		"the graph calls the middle name a patronymic")
}

// TestSessionCookieTokenIsAcceptedLive is the assertion behind the auth fast
// path: familio's own `t` cookie value, sent as the bearer, is accepted by the
// authed API. It only runs when the credentials arrived as a cookie (the browser
// and session-token paths produce the same `t`).
func TestSessionCookieTokenIsAcceptedLive(t *testing.T) {
	if os.Getenv("FAMILIO_NETWORK_TEST") != "1" {
		t.Skip("set FAMILIO_NETWORK_TEST=1 to run the live familio.org decode test")
	}
	RegisterTestingT(t)

	client := newLiveClient(t)
	token, exp, uuid, ok := client.sessionCookieToken()
	if !ok {
		t.Skip("the session cookie does not carry a usable JWT — the scrape fallback is in use")
	}
	Expect(token).To(HavePrefix("eyJ"))
	Expect(uuid).ToNot(BeEmpty())
	Expect(exp).To(BeTemporally(">", time.Now()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// No scrape happened, so this call proves the cookie's own JWT authenticates.
	profile, err := client.GetProfile(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(profile.User.UUID).To(Equal(uuid))
	t.Logf("cookie JWT accepted; expires %s", exp.Format(time.RFC3339))
}

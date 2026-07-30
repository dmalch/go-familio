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

	var edges int
	for _, node := range graph.Nodes {
		Expect(node.NodeID).ToNot(BeEmpty())
		for _, edge := range append(append([]TreeGraphEdge{}, node.Params.Parents...), node.Params.Partners...) {
			Expect(edge.NodeID).ToNot(BeEmpty())
			// A sex that is neither literal means the layout hint changed shape.
			Expect([]string{GenderMale, GenderFemale}).To(ContainElement(edge.Sex),
				"unexpected sex %q on an edge of node %s", edge.Sex, node.NodeID)
			edges++
		}
	}
	t.Logf("decoded %d node(s) with %d edge(s)", len(graph.Nodes), edges)

	// The node ids are person uuids: the first one must resolve.
	basic, err := client.GetPersonBasic(ctx, graph.Nodes[0].NodeID)
	Expect(err).ToNot(HaveOccurred())
	Expect(basic.UUID).To(Equal(graph.Nodes[0].NodeID))
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

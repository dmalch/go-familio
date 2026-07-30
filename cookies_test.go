package familio

import (
	"net/url"
	"testing"

	. "github.com/onsi/gomega"
)

func TestCookiesFromHeader(t *testing.T) {
	RegisterTestingT(t)
	cookies := CookiesFromHeader("t=abc123;  other=xyz ; =skip; bad")
	Expect(cookies).To(HaveLen(2))
	Expect(cookies[0].Name).To(Equal("t"))
	Expect(cookies[0].Value).To(Equal("abc123"))
	Expect(cookies[1].Name).To(Equal("other"))
	Expect(cookies[1].Value).To(Equal("xyz"))
}

func TestCookiesFromHeaderEmpty(t *testing.T) {
	RegisterTestingT(t)
	Expect(CookiesFromHeader("   ")).To(BeNil())
}

func TestCookieFromSessionToken(t *testing.T) {
	RegisterTestingT(t)
	cookies := CookieFromSessionToken("  tok  ")
	Expect(cookies).To(HaveLen(1))
	Expect(cookies[0].Name).To(Equal("t"))
	Expect(cookies[0].Value).To(Equal("tok"))
	Expect(CookieFromSessionToken("")).To(BeNil())
}

// TestCookieValueIsEncodedWhenItMustBe locks the wire form: familio's `t` cookie
// holds a JSON object, and net/http silently drops bytes that are illegal in a
// cookie value — so the credential has to be percent-encoded, exactly as a
// browser sends it. Values that are already legal must be left alone.
func TestCookieValueIsEncodedWhenItMustBe(t *testing.T) {
	RegisterTestingT(t)
	const envelope = `{"token":"eyJhbG.eyJleHA.sig","synapseToken":"syt_abc"}`

	got := CookieFromSessionToken(envelope)
	Expect(got).To(HaveLen(1))
	Expect(got[0].Value).ToNot(ContainSubstring(`"`), "quotes would be dropped in transit")
	Expect(got[0].Value).To(ContainSubstring("%22"))

	// It must survive the round-trip back to the original value.
	decoded, err := url.QueryUnescape(got[0].Value)
	Expect(err).ToNot(HaveOccurred())
	Expect(decoded).To(Equal(envelope))

	// A bare JWT is already legal: unchanged, never double-encoded.
	bare := CookieFromSessionToken("eyJhbG.eyJleHA.sig")
	Expect(bare[0].Value).To(Equal("eyJhbG.eyJleHA.sig"))

	// Same rule via the header parser.
	header := CookiesFromHeader("t=" + envelope + "; other=plain")
	Expect(header).To(HaveLen(2))
	Expect(header[0].Value).To(ContainSubstring("%22"))
	Expect(header[1].Value).To(Equal("plain"))
}

// TestCookiesFromHeaderKeepsAnAlreadyEncodedValue is the DevTools case: a header
// copied from the Network panel is already percent-encoded, and re-encoding it
// would break the credential.
func TestCookiesFromHeaderKeepsAnAlreadyEncodedValue(t *testing.T) {
	RegisterTestingT(t)
	const encoded = `%7B%22token%22%3A%22eyJhbG.eyJleHA.sig%22%7D`

	got := CookiesFromHeader("t=" + encoded)
	Expect(got).To(HaveLen(1))
	Expect(got[0].Value).To(Equal(encoded), "an already-encoded value must not be encoded again")
}

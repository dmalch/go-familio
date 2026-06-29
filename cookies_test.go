package familio

import (
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

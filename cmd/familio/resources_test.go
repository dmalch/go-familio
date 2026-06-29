package main

import (
	"bytes"
	"testing"

	. "github.com/onsi/gomega"
)

func TestResolveCookies_FlagWinsAsHeader(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("FAMILIO_SESSION", "ignored-because-flag-set")
	got, err := resolveCookies(&globalOpts{cookies: "t=abc; other=def"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(HaveLen(2))
	g.Expect(got[0].Name).To(Equal("t"))
	g.Expect(got[0].Value).To(Equal("abc"))
}

func TestResolveCookies_SessionTokenEnv(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("FAMILIO_SESSION", "session-xyz")
	got, err := resolveCookies(&globalOpts{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(HaveLen(1))
	g.Expect(got[0].Name).To(Equal("t"))
	g.Expect(got[0].Value).To(Equal("session-xyz"))
}

func TestResolveCookies_NothingConfiguredIsNil(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("FAMILIO_SESSION", "")
	got, err := resolveCookies(&globalOpts{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(BeNil())
}

func TestOneArg(t *testing.T) {
	g := NewWithT(t)

	v, err := oneArg([]string{"u1"}, "uuid")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(v).To(Equal("u1"))

	_, err = oneArg(nil, "uuid")
	g.Expect(err).To(HaveOccurred())

	_, err = oneArg([]string{"a", "b"}, "uuid")
	g.Expect(err).To(HaveOccurred())

	_, err = oneArg([]string{""}, "uuid")
	g.Expect(err).To(HaveOccurred())
}

func TestRender_WritesIndentedJSONWithoutHTMLEscaping(t *testing.T) {
	g := NewWithT(t)
	var buf bytes.Buffer
	err := render(&buf, map[string]string{"name": "Иван & Co"})
	g.Expect(err).ToNot(HaveOccurred())
	out := buf.String()
	g.Expect(out).To(ContainSubstring("\"name\": \"Иван & Co\""))
	g.Expect(out).ToNot(ContainSubstring("\\u0026")) // HTML escaping disabled
}

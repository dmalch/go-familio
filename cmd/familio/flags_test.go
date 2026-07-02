package main

import (
	"bytes"
	"flag"
	"testing"

	. "github.com/onsi/gomega"
)

// TestParseFlags_FlagsAnywhere proves flags are parsed before, between, and
// after positional arguments (issue #8).
func TestParseFlags_FlagsAnywhere(t *testing.T) {
	g := NewWithT(t)
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var x string
	var b bool
	fs.StringVar(&x, "x", "", "")
	fs.BoolVar(&b, "b", false, "")

	pos, err := parseFlags(fs, []string{"pos1", "-x", "val", "pos2", "-b"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pos).To(Equal([]string{"pos1", "pos2"}))
	g.Expect(x).To(Equal("val"))
	g.Expect(b).To(BeTrue())
}

// TestParseOneUUID_GlobalFlagAfterArg is the concrete case from issue #8:
// "person get <uuid> -browser chrome" must set the global flag and still yield
// the uuid.
func TestParseOneUUID_GlobalFlagAfterArg(t *testing.T) {
	g := NewWithT(t)
	var errb bytes.Buffer
	opts := &globalOpts{stderr: &errb}
	uuid, done, err := opts.parseOneUUID("person get", "uuid", []string{"the-uuid", "-browser", "chrome"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(done).To(BeFalse())
	g.Expect(uuid).To(Equal("the-uuid"))
	g.Expect(opts.browser).To(Equal("chrome"))
}

// TestParseOneUUID_GlobalFlagDefaultsSurvive proves a value already resolved on
// globalOpts (from top-level flags/env) is kept when the flag is not repeated.
func TestParseOneUUID_GlobalFlagDefaultsSurvive(t *testing.T) {
	g := NewWithT(t)
	var errb bytes.Buffer
	opts := &globalOpts{stderr: &errb, cookies: "t=pre"}
	uuid, _, err := opts.parseOneUUID("person get", "uuid", []string{"the-uuid"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(uuid).To(Equal("the-uuid"))
	g.Expect(opts.cookies).To(Equal("t=pre"))
}

func TestTreeDirection(t *testing.T) {
	g := NewWithT(t)

	d, err := treeDirection(false, false, false)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(d).To(Equal("component"))

	d, _ = treeDirection(true, false, false)
	g.Expect(d).To(Equal("up"))

	d, _ = treeDirection(false, true, false)
	g.Expect(d).To(Equal("down"))

	_, err = treeDirection(true, true, false)
	g.Expect(err).To(HaveOccurred())
}

func TestParseDate(t *testing.T) {
	g := NewWithT(t)

	r, err := parseDate("1900-05-12")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(r.Year).To(Equal(1900))
	g.Expect(*r.Month).To(Equal(5))
	g.Expect(*r.Day).To(Equal(12))

	r, err = parseDate("1826")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(r.Year).To(Equal(1826))
	g.Expect(r.Month).To(BeNil())

	_, err = parseDate("not-a-date")
	g.Expect(err).To(HaveOccurred())

	_, err = parseDate("1900-05-12-99")
	g.Expect(err).To(HaveOccurred())
}

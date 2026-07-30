package main

import (
	"bytes"
	"encoding/json"
	"testing"

	familio "github.com/dmalch/go-familio"
	. "github.com/onsi/gomega"
)

// TestTagWriteFlags_Input checks the write flags land on the library input.
func TestTagWriteFlags_Input(t *testing.T) {
	g := NewWithT(t)
	fs := (&globalOpts{stderr: &bytes.Buffer{}}).newFlagSet("tags create")
	var w tagWriteFlags
	w.register(fs)
	_, err := parseFlags(fs, []string{
		"-name", "Проверить в архиве",
		"-color", "mint-mist",
		"-description", "Нужен запрос в ЦГА",
	})
	g.Expect(err).ToNot(HaveOccurred())

	in, err := w.input()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(in).To(Equal(familio.TagInput{
		Name:        "Проверить в архиве",
		Color:       familio.TagColorMintMist,
		Description: "Нужен запрос в ЦГА",
	}))
}

// TestTagWriteFlags_Validation checks the CLI surfaces the library's rules
// rather than reimplementing them.
func TestTagWriteFlags_Validation(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no name", []string{"-color", "mint-mist"}, "tag name is required"},
		{"no color", []string{"-name", "Метка"}, "invalid tag color"},
		{"unknown color", []string{"-name", "Метка", "-color", "puce"}, `invalid tag color "puce"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			fs := (&globalOpts{stderr: &bytes.Buffer{}}).newFlagSet("tags create")
			var w tagWriteFlags
			w.register(fs)
			_, err := parseFlags(fs, tc.args)
			g.Expect(err).ToNot(HaveOccurred())
			_, err = w.input()
			g.Expect(err).To(MatchError(ContainSubstring(tc.want)))
		})
	}
}

// TestRun_TagsColors checks the palette command prints every code with its hex
// and makes no request — it succeeds with no credentials configured.
func TestRun_TagsColors(t *testing.T) {
	g := NewWithT(t)
	code, out, errb := runArgs("tags", "colors")
	g.Expect(code).To(Equal(0), "stderr: %s", errb)

	var colors []tagColor
	g.Expect(json.Unmarshal([]byte(out), &colors)).To(Succeed())
	g.Expect(colors).To(HaveLen(len(familio.TagColors)))
	g.Expect(colors[0].Color).To(Equal(familio.TagColorRoseMist))
	for _, c := range colors {
		g.Expect(familio.TagColorHex[c.Color]).To(Equal(c.Hex))
	}
}

// TestRun_TagsColors_RejectsPositionals keeps the local command strict too.
func TestRun_TagsColors_RejectsPositionals(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("tags", "colors", "mint-mist")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("takes no positional arguments"))
}

// TestRun_TagsList_RejectsPositionals checks the list command takes none.
func TestRun_TagsList_RejectsPositionals(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("tags", "list", "some-uuid")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("takes no positional arguments"))
}

// TestRun_TagsCommands_RequireArguments checks each command names what it wants
// before it reaches the network.
func TestRun_TagsCommands_RequireArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"person", []string{"tags", "person"}, "expected exactly one <person-uuid> argument"},
		{"by-persons", []string{"tags", "by-persons"}, "expected at least one <person-uuid> argument"},
		{"delete", []string{"tags", "delete"}, "expected at least one <tag-id> argument"},
		{"assign no args", []string{"tags", "assign"}, "expected <person-uuid> followed by at least one <tag-id>"},
		{"assign no tags", []string{"tags", "assign", "p-1"}, "expected <person-uuid> followed by at least one <tag-id>"},
		{"unassign no tags", []string{"tags", "unassign", "p-1"}, "expected <person-uuid> followed by at least one <tag-id>"},
		{"update no id", []string{"tags", "update", "-name", "x", "-color", "mint-mist"}, "expected exactly one <tag-id> argument"},
		{"create with positional", []string{"tags", "create", "extra", "-name", "x", "-color", "mint-mist"}, "takes no positional arguments"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			code, _, errb := runArgs(tc.args...)
			g.Expect(code).To(Equal(1))
			g.Expect(errb).To(ContainSubstring(tc.want))
		})
	}
}

// TestParseTagID covers the one shape surprise in the tags API: a tag is keyed
// by a small positive integer, not a uuid, so the CLI names that mistake
// instead of passing a uuid through to a confusing server error.
func TestParseTagID(t *testing.T) {
	g := NewWithT(t)
	id, err := parseTagID(" 2832 ")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(id).To(Equal(2832))

	ids, err := parseTagIDs([]string{"1", "2832"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ids).To(Equal([]int{1, 2832}))

	for _, bad := range []string{"", "0", "-1", "abc", "8f5c3f1e-1d0a-4b6f-9b52-4a25b3f0a1c7", "2832.5"} {
		_, err := parseTagID(bad)
		g.Expect(err).To(MatchError(ContainSubstring("want a positive integer")), "input %q", bad)
	}

	// A bad id anywhere in the list fails the whole list.
	_, err = parseTagIDs([]string{"2832", "nope"})
	g.Expect(err).To(MatchError(ContainSubstring(`invalid <tag-id> "nope"`)))
}

// TestRun_TagsCommands_RejectUUIDTagIDs checks the id-shape error reaches the
// user from every command that takes tag ids — and, for the mutating ones,
// before the confirmation prompt, so a typo never gets a "Proceed?".
func TestRun_TagsCommands_RejectUUIDTagIDs(t *testing.T) {
	const uuid = "8f5c3f1e-1d0a-4b6f-9b52-4a25b3f0a1c7"
	for _, args := range [][]string{
		{"tags", "delete", uuid},
		{"tags", "assign", "p-1", uuid},
		{"tags", "unassign", "p-1", uuid},
		{"tags", "update", uuid, "-name", "x", "-color", "mint-mist"},
	} {
		g := NewWithT(t)
		code, _, errb := runArgsStdin("y\n", args...)
		g.Expect(code).To(Equal(1))
		g.Expect(errb).To(ContainSubstring("want a positive integer"), "args %v", args)
		g.Expect(errb).ToNot(ContainSubstring("Proceed?"), "args %v", args)
	}
}

// TestRun_TagsWrites_ValidateBeforeAuth checks the create/update commands reject
// bad input before a client is built, so no request is ever attempted.
func TestRun_TagsWrites_ValidateBeforeAuth(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("tags", "create", "-name", "Метка", "-color", "puce")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring(`invalid tag color "puce"`))

	code, _, errb = runArgs("tags", "update", "2832", "-name", "", "-color", "mint-mist")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("tag name is required"))
}

// TestRun_TagsDelete_AbortsOnNo checks that declining the prompt fails the
// command before any request is made.
func TestRun_TagsDelete_AbortsOnNo(t *testing.T) {
	g := NewWithT(t)
	code, out, errb := runArgsStdin("n\n", "tags", "delete", "2832", "2833")
	g.Expect(code).To(Equal(1))
	g.Expect(out).To(BeEmpty())
	g.Expect(errb).To(ContainSubstring("Delete 2 tag(s):"))
	g.Expect(errb).To(ContainSubstring("2832"))
	g.Expect(errb).To(ContainSubstring("2833"))
	g.Expect(errb).To(ContainSubstring("delete aborted"))
}

// TestRun_TagsUnassign_AbortsOnNo checks the unassign prompt names the person
// the tags come off, since the tag ids alone do not say.
func TestRun_TagsUnassign_AbortsOnNo(t *testing.T) {
	g := NewWithT(t)
	code, out, errb := runArgsStdin("n\n", "tags", "unassign", "p-1", "2832")
	g.Expect(code).To(Equal(1))
	g.Expect(out).To(BeEmpty())
	g.Expect(errb).To(ContainSubstring("Unassign 1 tag(s) on person p-1:"))
	g.Expect(errb).To(ContainSubstring("unassign aborted"))
}

// TestRun_TagsAssign_AbortsOnEmptyStdin checks the safe default: an empty or
// closed stdin declines rather than proceeding.
func TestRun_TagsAssign_AbortsOnEmptyStdin(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgsStdin("", "tags", "assign", "p-1", "2832")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("assign aborted"))
}

// TestRun_Help_ListsTagsCommands checks the group is wired into the tree.
func TestRun_Help_ListsTagsCommands(t *testing.T) {
	g := NewWithT(t)
	code, out, _ := runArgs("help")
	g.Expect(code).To(Equal(0))
	for _, want := range []string{
		"tags list", "tags person", "tags by-persons", "tags colors",
		"tags create", "tags update", "tags delete", "tags assign", "tags unassign",
	} {
		g.Expect(out).To(ContainSubstring(want))
	}
}

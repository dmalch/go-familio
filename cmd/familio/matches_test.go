package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	familio "github.com/dmalch/go-familio"
	. "github.com/onsi/gomega"
)

// runArgsStdin is runArgs with a scripted stdin, for the commands that prompt.
func runArgsStdin(stdin string, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

// TestMatchesListFlags_Filter checks that every filter flag lands on the
// library filter, including the repeatable ones.
func TestMatchesListFlags_Filter(t *testing.T) {
	g := NewWithT(t)
	fs := (&globalOpts{stderr: &bytes.Buffer{}}).newFlagSet("matches list")
	var m matchesListFlags
	m.register(fs)
	_, err := parseFlags(fs, []string{
		"-person", "p-1", "-person", "p-2",
		"-user", "u-1",
		"-catalog", "vss", "-catalog", "blockade",
		"-date", "2026-07-20",
		"-status", "undecided", "-status", "confirmed",
		"-min-score", "90", "-max-score", "95",
		"-page", "3", "-limit", "50",
	})
	g.Expect(err).ToNot(HaveOccurred())

	f, err := m.filter()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(f).To(Equal(familio.MatchFilter{
		PersonIDs:    []string{"p-1", "p-2"},
		UserIDs:      []string{"u-1"},
		Catalogs:     []string{"vss", "blockade"},
		Dates:        []string{"2026-07-20"},
		Statuses:     []string{familio.MatchStatusUndecided, familio.MatchStatusConfirmed},
		MinScore:     90,
		MaxScore:     95,
		Page:         3,
		ItemsPerPage: 50,
	}))
}

// TestMatchesListFlags_Defaults checks the no-flag case maps to the full score
// window and the UI's page size, with paging left at the library default.
func TestMatchesListFlags_Defaults(t *testing.T) {
	g := NewWithT(t)
	fs := (&globalOpts{stderr: &bytes.Buffer{}}).newFlagSet("matches list")
	var m matchesListFlags
	m.register(fs)
	_, err := parseFlags(fs, nil)
	g.Expect(err).ToNot(HaveOccurred())

	f, err := m.filter()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(f).To(Equal(familio.MatchFilter{MinScore: 1, MaxScore: 99, ItemsPerPage: 15}))
}

// TestMatchesListFlags_Validation covers every rejected flag combination.
func TestMatchesListFlags_Validation(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown status", []string{"-status", "pending"}, `invalid -status "pending"`},
		{"min score too low", []string{"-min-score", "0"}, "between 1 and 99"},
		{"max score too high", []string{"-max-score", "100"}, "between 1 and 99"},
		{"inverted window", []string{"-min-score", "90", "-max-score", "80"}, "must not exceed"},
		{"all with page", []string{"-all", "-page", "2"}, "-all cannot be combined with -page"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			fs := (&globalOpts{stderr: &bytes.Buffer{}}).newFlagSet("matches list")
			var m matchesListFlags
			m.register(fs)
			_, err := parseFlags(fs, tc.args)
			g.Expect(err).ToNot(HaveOccurred())
			_, err = m.filter()
			g.Expect(err).To(MatchError(ContainSubstring(tc.want)))
		})
	}
}

// TestConfirmAction covers the prompt's answer handling. Anything but an
// explicit yes declines, so the safe answer is the default — including on EOF,
// which is what an empty or closed stdin yields.
func TestConfirmAction(t *testing.T) {
	for _, tc := range []struct {
		stdin string
		want  bool
	}{
		{"y\n", true}, {"Y\n", true}, {"yes\n", true}, {" y \n", true},
		{"n\n", false}, {"no\n", false}, {"\n", false}, {"", false}, {"maybe\n", false},
	} {
		g := NewWithT(t)
		var errb bytes.Buffer
		gopts := &globalOpts{stdin: strings.NewReader(tc.stdin), stderr: &errb}
		g.Expect(confirmAction(gopts, "reject", []string{"m-1"})).To(Equal(tc.want), "stdin %q", tc.stdin)
	}
}

// TestConfirmAction_PromptsOnStderr checks the prompt names the verb and lists
// the affected matches, on stderr so stdout stays pure JSON.
func TestConfirmAction_PromptsOnStderr(t *testing.T) {
	g := NewWithT(t)
	var errb bytes.Buffer
	gopts := &globalOpts{stdin: strings.NewReader("n\n"), stderr: &errb}
	g.Expect(confirmAction(gopts, "reject", []string{"m-1", "m-2"})).To(BeFalse())
	g.Expect(errb.String()).To(ContainSubstring("Reject 2 match(es):"))
	g.Expect(errb.String()).To(ContainSubstring("m-1"))
	g.Expect(errb.String()).To(ContainSubstring("m-2"))
	g.Expect(errb.String()).To(ContainSubstring("[y/N]:"))
}

// TestRun_MatchesReject_AbortsOnNo checks that declining the prompt fails the
// command before any request is made — no credentials are configured here, so
// reaching the client at all would surface a different error.
func TestRun_MatchesReject_AbortsOnNo(t *testing.T) {
	g := NewWithT(t)
	code, out, errb := runArgsStdin("n\n", "matches", "reject", "95e794df-4edb-4661-8bd9-61fd3fc6d52e")
	g.Expect(code).To(Equal(1))
	g.Expect(out).To(BeEmpty())
	g.Expect(errb).To(ContainSubstring("95e794df-4edb-4661-8bd9-61fd3fc6d52e"))
	g.Expect(errb).To(ContainSubstring("reject aborted"))
}

// TestRun_MatchesDecisions_RequireUUIDs checks the three status commands all
// reject an empty argument list.
func TestRun_MatchesDecisions_RequireUUIDs(t *testing.T) {
	for _, verb := range []string{"confirm", "reject", "undecide"} {
		g := NewWithT(t)
		code, _, errb := runArgs("matches", verb)
		g.Expect(code).To(Equal(1))
		g.Expect(errb).To(ContainSubstring("expected at least one <match-uuid> argument"))
	}
}

// TestRun_MatchesList_RejectsPositionals checks the list and filters commands
// take no positional arguments.
func TestRun_MatchesList_RejectsPositionals(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("matches", "list", "some-uuid")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("takes no positional arguments"))

	code, _, errb = runArgs("matches", "filters", "some-uuid")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("takes no positional arguments"))
}

// TestRun_MatchesList_ReportsInvalidFlagsBeforeAuth checks that flag validation
// happens before the client is built, so a bad flag never triggers a request.
func TestRun_MatchesList_ReportsInvalidFlagsBeforeAuth(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("matches", "list", "-status", "pending")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring(`invalid -status "pending"`))
}

// TestRun_Help_ListsMatchesCommands checks the group is wired into the tree.
func TestRun_Help_ListsMatchesCommands(t *testing.T) {
	g := NewWithT(t)
	code, out, _ := runArgs("help")
	g.Expect(code).To(Equal(0))
	for _, want := range []string{
		"matches list", "matches filters", "matches confirm", "matches reject", "matches undecide",
	} {
		g.Expect(out).To(ContainSubstring(want))
	}
}

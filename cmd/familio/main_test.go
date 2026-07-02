package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// runArgs is a test helper: it runs the CLI with args (no stdin) and
// returns the exit code plus captured stdout and stderr.
func runArgs(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func TestRun_Help_PrintsUsageToStdout(t *testing.T) {
	g := NewWithT(t)
	code, out, _ := runArgs("help")
	g.Expect(code).To(Equal(0))
	g.Expect(out).To(ContainSubstring("command-line client for the familio.org API"))
	g.Expect(out).To(ContainSubstring("person get"))
	g.Expect(out).To(ContainSubstring("settlement persons"))
}

func TestRun_NoArgs_IsUsageError(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs()
	g.Expect(code).To(Equal(2))
	g.Expect(errb).To(ContainSubstring("Usage:"))
}

func TestRun_UnknownCommand_IsUsageError(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("frobnicate")
	g.Expect(code).To(Equal(2))
	g.Expect(errb).To(ContainSubstring("unknown command"))
}

func TestRun_GroupWithoutSubcommand_IsUsageError(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("person")
	g.Expect(code).To(Equal(2))
	g.Expect(errb).To(ContainSubstring("expected a subcommand"))
}

func TestRun_UnknownSubcommand_IsUsageError(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("person", "frobnicate")
	g.Expect(code).To(Equal(2))
	g.Expect(errb).To(ContainSubstring("unknown subcommand"))
}

func TestRun_MissingArg_IsCommandError(t *testing.T) {
	g := NewWithT(t)
	// settlement get with no uuid: fails in the handler before any network
	// call, so it is a command error (exit 1), not a usage error.
	code, _, errb := runArgs("settlement", "get")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("expected exactly one <uuid>"))
}

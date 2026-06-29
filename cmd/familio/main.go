// Command familio is a read-only command-line client for the familio.org
// genealogy API.
//
// It is a thin façade over the github.com/dmalch/go-familio library: the
// read commands ("familio person get", "familio settlement get",
// "familio whoami", …) print JSON results to stdout. Authentication reuses
// the same credential sources as the Terraform provider — FAMILIO_COOKIES,
// FAMILIO_SESSION, or a logged-in browser via -browser.
//
// Run "familio help" for the full command list.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// globalOpts holds the flags and I/O streams shared by every command.
type globalOpts struct {
	cookies string // -cookies / FAMILIO_COOKIES — raw "name=value; …" header
	browser string // -browser / FAMILIO_BROWSER — read cookies from a logged-in browser
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

// command is a node in the command tree: a leaf when run is set, a group
// of subcommands when sub is set.
type command struct {
	summary string
	run     func(ctx context.Context, g *globalOpts, args []string) error
	sub     map[string]*command
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run parses args, dispatches to a command, and returns a process exit
// code: 0 success, 1 command error, 2 usage error.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	g := &globalOpts{stdin: stdin, stdout: stdout, stderr: stderr}

	fs := flag.NewFlagSet("familio", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&g.cookies, "cookies", "", "session cookies as a raw \"name=value; …\" header (overrides FAMILIO_COOKIES)")
	fs.StringVar(&g.browser, "browser", "", "read cookies from a logged-in browser (chrome|edge|brave|chromium|vivaldi|opera|firefox|safari); empty = env vars only")
	fs.Usage = func() { printUsage(stderr) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if g.cookies == "" {
		g.cookies = os.Getenv("FAMILIO_COOKIES")
	}
	if g.browser == "" {
		g.browser = os.Getenv("FAMILIO_BROWSER")
	}

	rest := fs.Args()
	if len(rest) == 0 {
		printUsage(stderr)
		return 2
	}

	cmd, ok := commandTree()[rest[0]]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "familio: unknown command %q\n\n", rest[0])
		printUsage(stderr)
		return 2
	}
	path := rest[0]
	rest = rest[1:]

	for cmd.sub != nil {
		if len(rest) == 0 {
			_, _ = fmt.Fprintf(stderr, "familio %s: expected a subcommand\n\n", path)
			printUsage(stderr)
			return 2
		}
		next, ok := cmd.sub[rest[0]]
		if !ok {
			_, _ = fmt.Fprintf(stderr, "familio %s: unknown subcommand %q\n\n", path, rest[0])
			printUsage(stderr)
			return 2
		}
		cmd = next
		path += " " + rest[0]
		rest = rest[1:]
	}

	if cmd.run == nil {
		printUsage(stderr)
		return 2
	}
	if err := cmd.run(ctx, g, rest); err != nil {
		_, _ = fmt.Fprintf(stderr, "familio %s: %v\n", path, err)
		return 1
	}
	return 0
}

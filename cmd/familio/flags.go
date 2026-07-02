package main

import (
	"errors"
	"flag"
)

// newFlagSet builds a per-command FlagSet that carries the shared global flags
// (-cookies/-browser) in addition to any command-specific flags the caller adds.
// The global flags default to the values already resolved on g (top-level flags
// + env), so a credential given before the subcommand survives and one given
// after it overrides. This is what lets global flags appear after the
// subcommand and its arguments (issue #8).
func (g *globalOpts) newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(g.stderr)
	fs.StringVar(&g.cookies, "cookies", g.cookies, "session cookies as a raw \"name=value; …\" header (overrides FAMILIO_COOKIES)")
	fs.StringVar(&g.browser, "browser", g.browser, "read cookies from a logged-in browser (chrome|edge|brave|chromium|vivaldi|opera|firefox|safari)")
	return fs
}

// parseFlags parses args allowing flags to appear before, between, or after the
// positional arguments — Go's flag package otherwise stops at the first
// positional, so "person get <uuid> -browser chrome" would drop the flag
// (issue #8). It returns the positional arguments. flag.ErrHelp is reported to
// the user by the FlagSet already, so it is returned as a nil-positionals,
// nil-error handled case is left to the caller via errors.Is.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positionals = append(positionals, args[0])
		args = args[1:]
	}
	return positionals, nil
}

// isHelp reports whether err is the flag package's -h/-help sentinel, which the
// FlagSet has already handled by printing usage. Callers treat it as a
// successful no-op rather than a command error.
func isHelp(err error) bool {
	return errors.Is(err, flag.ErrHelp)
}

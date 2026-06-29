package familio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/steipete/sweetcookie"
)

var (
	// ErrNoCookies is returned when no familio.org cookies were found in any
	// browser store on the host. Log in to familio.org in a browser first.
	ErrNoCookies = errors.New("familio: no familio.org cookies found in any browser")

	// ErrFullDiskAccessRequired wraps macOS "operation not permitted" failures
	// reading browser cookie stores (e.g. Safari's container needs Full Disk
	// Access). Grant it in System Settings → Privacy & Security → Full Disk
	// Access for the binary running this code.
	ErrFullDiskAccessRequired = errors.New(
		"familio: cannot read browser cookie store (on macOS, grant Full Disk " +
			"Access in System Settings → Privacy & Security)")
)

// SupportedBrowsers lists the browser names accepted by CookiesFromBrowser.
var SupportedBrowsers = []string{
	"chrome", "edge", "brave", "arc", "chromium",
	"vivaldi", "opera", "firefox", "safari",
}

// readCookies is the sweetcookie entry point, indirected for tests.
var readCookies = func(browsers []sweetcookie.Browser) (sweetcookie.Result, error) {
	return sweetcookie.Get(context.Background(), sweetcookie.Options{
		URL:      defaultBaseURL,
		Browsers: browsers,
	})
}

// CookiesFromBrowser reads valid (non-expired) familio.org cookies from the
// host's browser stores. With no arguments, sweetcookie's default browser
// priority is used; with names, only those backends are queried in order.
// Mirrors go-geni's browsercookies.FromGeniCom.
func CookiesFromBrowser(browsers ...string) ([]*http.Cookie, error) {
	bs, err := parseBrowsers(browsers)
	if err != nil {
		return nil, err
	}
	res, err := readCookies(bs)
	if err != nil {
		if isPermissionDenied(err) {
			return nil, fmt.Errorf("%w: %w", ErrFullDiskAccessRequired, err)
		}
		return nil, err
	}
	if len(res.Cookies) == 0 {
		return nil, ErrNoCookies
	}
	return toHTTPCookies(res.Cookies), nil
}

func parseBrowsers(names []string) ([]sweetcookie.Browser, error) {
	if len(names) == 0 {
		return nil, nil
	}
	out := make([]sweetcookie.Browser, 0, len(names))
	for _, n := range names {
		norm := strings.ToLower(strings.TrimSpace(n))
		if norm == "" {
			continue
		}
		if !slices.Contains(SupportedBrowsers, norm) {
			return nil, fmt.Errorf("familio: unknown browser %q (supported: %s)",
				n, strings.Join(SupportedBrowsers, ", "))
		}
		out = append(out, sweetcookie.Browser(norm))
	}
	return out, nil
}

func toHTTPCookies(in []sweetcookie.Cookie) []*http.Cookie {
	out := make([]*http.Cookie, len(in))
	for i, c := range in {
		hc := &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			HttpOnly: c.HTTPOnly,
			Secure:   c.Secure,
		}
		if c.Expires != nil {
			hc.Expires = *c.Expires
		}
		out[i] = hc
	}
	return out
}

func isPermissionDenied(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "permission denied")
}

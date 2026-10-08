package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

const apiUsage = "usage: familio api [-X method] [-f key=value]... [-F key=value]... [-H 'Name: value']... " +
	"[-input file] [-i] [-paginate] <endpoint>"

// ownerPlaceholder is replaced with the account uuid, the way gh fills {owner}
// with the current repository's owner: familio files its per-account
// collections under the owner's uuid (users/{owner}/tags, persons/history/{owner}).
const ownerPlaceholder = "{owner}"

// apiOpts holds the parsed "familio api" flags.
type apiOpts struct {
	method   string
	fields   []rawField
	headers  multiFlag
	input    string
	include  bool
	paginate bool
}

// apiCall is the request the flags describe.
type apiCall struct {
	method string
	fields []apiField
	body   []byte // from -input; nil when absent
	header http.Header
}

// runAPI handles "familio api [flags] <endpoint>" — a raw call to any familio.org
// /api/v2 endpoint in the spirit of "gh api", authenticated, rate limited and
// retried like every other command.
//
// Fields default the method to POST. On a GET, or when -input supplies the body,
// they go in the query string; otherwise they form a JSON object. The response
// body is printed as is, re-indented when it is JSON; a status outside 2xx
// prints the body too and exits 1.
func runAPI(ctx context.Context, g *globalOpts, args []string) error {
	fs := g.newFlagSet("api")
	// The long names are gh's, so "familio api" takes the same muscle memory.
	var o apiOpts
	fs.StringVar(&o.method, "X", "", "HTTP method (default GET, or POST when fields or -input are given)")
	fs.StringVar(&o.method, "method", "", "same as -X")
	fs.Var(fieldFlag{&o.fields, false}, "f", "add a string parameter key=value (repeatable)")
	fs.Var(fieldFlag{&o.fields, false}, "raw-field", "same as -f")
	fs.Var(fieldFlag{&o.fields, true}, "F",
		"add a typed parameter key=value: true, false, null and integers become JSON, @file reads a file, @- stdin, "+
			ownerPlaceholder+" is the account uuid (repeatable)")
	fs.Var(fieldFlag{&o.fields, true}, "field", "same as -F")
	fs.Var(&o.headers, "H", "add a request header 'Name: value' (repeatable)")
	fs.Var(&o.headers, "header", "same as -H")
	fs.StringVar(&o.input, "input", "", "read the request body from a file (- for stdin)")
	fs.BoolVar(&o.include, "i", false, "print the status line and response headers before the body")
	fs.BoolVar(&o.include, "include", false, "same as -i")
	fs.BoolVar(&o.paginate, "paginate", false, "follow the response's pager and print every page (GET or POST)")

	positional, err := parseFlags(fs, args)
	if err != nil {
		return ignoreHelp(err)
	}
	if len(positional) != 1 {
		return errors.New(apiUsage)
	}
	endpoint := positional[0]

	c, err := newClient(g)
	if err != nil {
		return err
	}
	if o.needsOwner(endpoint) {
		owner, err := c.AccountUUID(ctx)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", ownerPlaceholder, err)
		}
		endpoint = o.fillOwner(endpoint, owner)
	}

	call, err := o.call(g.stdin)
	if err != nil {
		return err
	}
	if o.paginate && call.method != http.MethodGet && call.method != http.MethodPost {
		return errors.New("-paginate resends the request for every page, so it needs a GET or a POST read")
	}

	path, rawQuery, _ := strings.Cut(endpoint, "?")
	body := call.body
	switch {
	case call.fieldsInQuery():
		rawQuery = joinQuery(rawQuery, apiFieldValues(call.fields).Encode())
	case len(call.fields) > 0:
		if body, err = apiJSONBody(call.fields); err != nil {
			return err
		}
	}

	for {
		target := path
		if rawQuery != "" {
			target += "?" + rawQuery
		}
		resp, err := c.DoRaw(ctx, call.method, target, call.header, body)
		if err != nil {
			return err
		}
		if err := writeAPIResponse(g.stdout, resp.StatusCode, resp.Header, resp.Body, o.include); err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		}
		if !o.paginate {
			return nil
		}
		next, err := nextPageQuery(rawQuery, resp.Body)
		if err != nil || next == "" {
			return err
		}
		rawQuery = next
	}
}

// ownerField returns the value of a -F field that takes the {owner} placeholder:
// one typed on the command line, not read with @file or @- and not a raw -f.
func ownerField(f rawField) (key, value string, ok bool) {
	key, value, _ = strings.Cut(f.arg, "=")
	if !f.typed || strings.HasPrefix(value, "@") || !strings.Contains(value, ownerPlaceholder) {
		return "", "", false
	}
	return key, value, true
}

// needsOwner reports whether the endpoint or a -F value carries {owner}.
func (o apiOpts) needsOwner(endpoint string) bool {
	return strings.Contains(endpoint, ownerPlaceholder) ||
		slices.ContainsFunc(o.fields, func(f rawField) bool {
			_, _, ok := ownerField(f)
			return ok
		})
}

// fillOwner replaces {owner} with the account uuid in the endpoint, which it
// returns, and in the -F values that take it.
func (o *apiOpts) fillOwner(endpoint, owner string) string {
	for i, f := range o.fields {
		if key, value, ok := ownerField(f); ok {
			o.fields[i].arg = key + "=" + strings.ReplaceAll(value, ownerPlaceholder, owner)
		}
	}
	return strings.ReplaceAll(endpoint, ownerPlaceholder, owner)
}

// call resolves the flags into the request to send.
func (o apiOpts) call(stdin io.Reader) (apiCall, error) {
	var c apiCall
	if o.input == "-" && slices.ContainsFunc(o.fields, func(f rawField) bool {
		return f.typed && strings.HasSuffix(f.arg, "=@-")
	}) {
		return c, errors.New("stdin can only be read once: -input - and -F key=@- both need it")
	}

	fields, err := parseAPIFields(o.fields, stdin)
	if err != nil {
		return c, err
	}
	c.fields = fields

	if o.input != "" {
		if o.input == "-" {
			c.body, err = io.ReadAll(stdin)
		} else {
			c.body, err = os.ReadFile(o.input)
		}
		if err != nil {
			return c, fmt.Errorf("read -input: %w", err)
		}
		if c.body == nil {
			c.body = []byte{}
		}
	}

	c.header = http.Header{}
	for _, h := range o.headers {
		name, value, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return c, fmt.Errorf("header %q must be 'Name: value'", h)
		}
		c.header.Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}

	c.method = strings.ToUpper(o.method)
	if c.method == "" {
		c.method = http.MethodGet
		if len(c.fields) > 0 || c.body != nil {
			c.method = http.MethodPost
		}
	}
	return c, nil
}

// fieldsInQuery reports whether the fields belong in the query string
// rather than the body: they do on a GET, and whenever -input already
// supplies the body.
func (c apiCall) fieldsInQuery() bool {
	return c.method == http.MethodGet || c.method == http.MethodHead || c.body != nil
}

// apiJSONBody encodes fields as a JSON object, keeping Cyrillic and "<>&" as
// they are.
func apiJSONBody(fields []apiField) ([]byte, error) {
	obj, err := apiFieldsJSON(fields)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(obj); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func joinQuery(a, b string) string {
	if a == "" || b == "" {
		return a + b
	}
	return a + "&" + b
}

// apiPager is the union of familio's two pager envelopes: page numbers
// {page, itemsPerPage, totalItems} on most lists, and a cursor {lastItem,
// hasMore} on the scroll endpoints.
type apiPager struct {
	Page         *int    `json:"page"`
	ItemsPerPage *int    `json:"itemsPerPage"`
	TotalItems   *int    `json:"totalItems"`
	LastItem     *string `json:"lastItem"`
	HasMore      *bool   `json:"hasMore"`
}

// nextPageQuery returns the query for the page after the one in body, or ""
// when it was the last page or carries no pager. It errors when the server did
// not move to the page it was asked for, which would otherwise page forever.
func nextPageQuery(rawQuery string, body []byte) (string, error) {
	var page struct {
		Data  []json.RawMessage `json:"data"`
		Pager *apiPager         `json:"pager"`
	}
	// A body that is not a pager envelope (a bare array, say) is a single page.
	if err := json.Unmarshal(body, &page); err != nil || page.Pager == nil {
		return "", nil //nolint:nilerr // not paged, so there is no next page
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("-paginate: %w", err)
	}

	p := page.Pager
	switch {
	case p.HasMore != nil:
		if !*p.HasMore {
			return "", nil
		}
		if p.LastItem == nil || *p.LastItem == "" {
			return "", errors.New("-paginate: the pager has more but no lastItem to continue from")
		}
		if q.Get("pageAfterItem") == *p.LastItem {
			return "", fmt.Errorf("-paginate: the cursor did not move past %s", *p.LastItem)
		}
		q.Set("pageAfterItem", *p.LastItem)
	case p.Page != nil && p.ItemsPerPage != nil && p.TotalItems != nil:
		if asked, err := strconv.Atoi(q.Get("page")); err == nil && asked != *p.Page {
			return "", fmt.Errorf("-paginate: asked for page=%d but the server answered page %d", asked, *p.Page)
		}
		if len(page.Data) == 0 || *p.ItemsPerPage <= 0 || *p.Page**p.ItemsPerPage >= *p.TotalItems {
			return "", nil
		}
		q.Set("page", strconv.Itoa(*p.Page+1))
		q.Set("itemsPerPage", strconv.Itoa(*p.ItemsPerPage))
	default:
		return "", nil
	}
	return q.Encode(), nil
}

// writeAPIResponse prints a response: with include, a status line and
// the headers first, as "gh api -i" does; then the body, re-indented
// when it is JSON and byte for byte otherwise.
func writeAPIResponse(w io.Writer, status int, header http.Header, body []byte, include bool) error {
	if include {
		var b strings.Builder
		fmt.Fprintf(&b, "HTTP %d %s\n", status, http.StatusText(status))
		for _, name := range slices.Sorted(maps.Keys(header)) {
			for _, v := range header[name] {
				fmt.Fprintf(&b, "%s: %s\n", name, v)
			}
		}
		b.WriteString("\n")
		if _, err := io.WriteString(w, b.String()); err != nil {
			return err
		}
	}

	var indented bytes.Buffer
	if json.Valid(body) && indentJSON(&indented, body) == nil {
		indented.WriteByte('\n')
		body = indented.Bytes()
	}
	_, err := w.Write(body)
	return err
}

// indentJSON writes the JSON document in body to out indented the way
// json.Indent does, keeping the key order and the number literals, but with
// every string re-encoded: familio's API escapes each Cyrillic letter as \uXXXX,
// and json.Indent would keep those escapes. Only what JSON requires (quotes,
// backslashes, control characters) stays escaped.
func indentJSON(out *bytes.Buffer, body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	type container struct {
		object  bool
		empty   bool
		wantKey bool // in an object: the next string is a key
	}
	var stack []*container
	newline := func() {
		out.WriteByte('\n')
		out.WriteString(strings.Repeat("  ", len(stack)))
	}
	// valueDone records that a complete value was written to the container
	// holding it, so an object expects its next key.
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].wantKey = true
		}
	}

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		if d, ok := tok.(json.Delim); ok && (d == '}' || d == ']') {
			closed := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !closed.empty {
				newline()
			}
			out.WriteByte(byte(d))
			valueDone()
			continue
		}

		// Every array element and every object key starts on its own line.
		if n := len(stack); n > 0 && (!stack[n-1].object || stack[n-1].wantKey) {
			if !stack[n-1].empty {
				out.WriteByte(',')
			}
			stack[n-1].empty = false
			newline()
		}

		switch v := tok.(type) {
		case json.Delim:
			out.WriteByte(byte(v))
			stack = append(stack, &container{object: v == '{', empty: true, wantKey: v == '{'})
		case string:
			if err := enc.Encode(v); err != nil {
				return err
			}
			out.Truncate(out.Len() - 1) // Encode's trailing newline
			if n := len(stack); n > 0 && stack[n-1].wantKey {
				stack[n-1].wantKey = false
				out.WriteString(": ")
				continue
			}
			valueDone()
		case json.Number:
			out.WriteString(v.String())
			valueDone()
		case bool:
			out.WriteString(strconv.FormatBool(v))
			valueDone()
		case nil:
			out.WriteString("null")
			valueDone()
		}
	}
}

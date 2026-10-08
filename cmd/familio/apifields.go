package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

// rawField is one -f or -F argument as typed, kept in command-line order
// so that repeated "key[]" values append in the order they were given.
type rawField struct {
	arg   string
	typed bool // -F rather than -f
}

// fieldFlag is the flag.Value behind -f and -F: both append to the same
// slice, so their relative order survives.
type fieldFlag struct {
	fields *[]rawField
	typed  bool
}

func (f fieldFlag) String() string { return "" }

func (f fieldFlag) Set(v string) error {
	*f.fields = append(*f.fields, rawField{arg: v, typed: f.typed})
	return nil
}

// apiField is a parsed parameter: a string for -f, and for -F a bool,
// nil, int64 or string.
type apiField struct {
	key   string
	value any
}

// parseAPIFields splits each key=value and types the -F values the way
// gh does: true, false, null and integers become JSON literals, @file
// reads a file and @- reads stdin. Anything else stays a string.
func parseAPIFields(raw []rawField, stdin io.Reader) ([]apiField, error) {
	out := make([]apiField, 0, len(raw))
	stdinRead := false
	for _, r := range raw {
		key, value, ok := strings.Cut(r.arg, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("field %q must be key=value", r.arg)
		}
		if !r.typed {
			out = append(out, apiField{key, value})
			continue
		}
		v, err := typedFieldValue(value, stdin, &stdinRead)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
		out = append(out, apiField{key, v})
	}
	return out, nil
}

func typedFieldValue(value string, stdin io.Reader, stdinRead *bool) (any, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil //nolint:nilnil // nil is the JSON null that was asked for
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		return n, nil
	}
	path, ok := strings.CutPrefix(value, "@")
	if !ok {
		return value, nil
	}
	if path == "-" {
		if *stdinRead {
			return nil, errors.New("stdin can only be read once")
		}
		*stdinRead = true
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// apiFieldValues encodes fields as query parameters. Keys stay literal —
// "operation[]" is nested by PHP on familio's side.
func apiFieldValues(fields []apiField) url.Values {
	v := url.Values{}
	for _, f := range fields {
		v.Add(f.key, fieldString(f.value))
	}
	return v
}

func fieldString(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case int64:
		return strconv.FormatInt(v, 10)
	}
	return fmt.Sprint(v)
}

// apiFieldsJSON builds a JSON request object from fields, nesting
// "a[b][c]" into objects and appending "a[]" values to an array, as gh
// does.
func apiFieldsJSON(fields []apiField) (map[string]any, error) {
	root := map[string]any{}
	for _, f := range fields {
		path, err := fieldPath(f.key)
		if err != nil {
			return nil, err
		}
		if err := setFieldPath(root, path, f.key, f.value); err != nil {
			return nil, err
		}
	}
	return root, nil
}

// fieldPath splits "a[b][c]" into [a b c] and "a[]" into [a ""]. An empty
// segment is only allowed last: "a[][b]" (an array of objects) is not
// supported.
func fieldPath(key string) ([]string, error) {
	invalid := fmt.Errorf("invalid field key %q", key)
	name, rest, nested := strings.Cut(key, "[")
	if name == "" || strings.Contains(name, "]") {
		return nil, invalid
	}
	path := []string{name}
	if !nested {
		return path, nil
	}
	rest = "[" + rest
	for rest != "" {
		if rest[0] != '[' {
			return nil, invalid
		}
		seg, after, ok := strings.Cut(rest[1:], "]")
		if !ok || strings.Contains(seg, "[") {
			return nil, invalid
		}
		path = append(path, seg)
		rest = after
	}
	if slices.Contains(path[1:len(path)-1], "") {
		return nil, invalid
	}
	return path, nil
}

func setFieldPath(root map[string]any, path []string, key string, value any) error {
	appendValue := path[len(path)-1] == ""
	if appendValue {
		path = path[:len(path)-1]
	}
	conflict := fmt.Errorf("field %q conflicts with another field", key)

	cur := root
	for _, seg := range path[:len(path)-1] {
		existing, ok := cur[seg]
		if !ok {
			next := map[string]any{}
			cur[seg] = next
			cur = next
			continue
		}
		next, isMap := existing.(map[string]any)
		if !isMap {
			return conflict
		}
		cur = next
	}

	last := path[len(path)-1]
	existing, ok := cur[last]
	if appendValue {
		if !ok {
			cur[last] = []any{value}
			return nil
		}
		arr, isArr := existing.([]any)
		if !isArr {
			return conflict
		}
		cur[last] = append(arr, value)
		return nil
	}
	if ok {
		switch existing.(type) {
		case map[string]any, []any:
			return conflict
		}
		return fmt.Errorf("field %q given more than once", key)
	}
	cur[last] = value
	return nil
}

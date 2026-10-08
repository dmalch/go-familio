package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestParseAPIFields(t *testing.T) {
	t.Run("-f keeps every value a string", func(t *testing.T) {
		g := NewWithT(t)
		got, err := parseAPIFields([]rawField{{"a=true", false}, {"b=42", false}, {"c=x=y", false}}, nil)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal([]apiField{{"a", "true"}, {"b", "42"}, {"c", "x=y"}}))
	})

	t.Run("-F converts literals and keeps the rest strings", func(t *testing.T) {
		g := NewWithT(t)
		got, err := parseAPIFields([]rawField{
			{"t=true", true}, {"f=false", true}, {"n=null", true},
			{"i=-42", true}, {"s=Метка", true}, {"fl=1.5", true},
		}, nil)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal([]apiField{
			{"t", true}, {"f", false}, {"n", nil},
			{"i", int64(-42)}, {"s", "Метка"}, {"fl", "1.5"},
		}))
	})

	t.Run("-F @file reads the file and @- reads stdin", func(t *testing.T) {
		g := NewWithT(t)
		path := filepath.Join(t.TempDir(), "bio.txt")
		g.Expect(os.WriteFile(path, []byte("Родился в Тамбове\n"), 0o600)).To(Succeed())

		got, err := parseAPIFields([]rawField{{"text=@" + path, true}, {"comment=@-", true}},
			strings.NewReader("из stdin"))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal([]apiField{{"text", "Родился в Тамбове\n"}, {"comment", "из stdin"}}))
	})

	t.Run("stdin can only be read once", func(t *testing.T) {
		g := NewWithT(t)
		_, err := parseAPIFields([]rawField{{"a=@-", true}, {"b=@-", true}}, strings.NewReader("x"))

		g.Expect(err).To(MatchError(ContainSubstring("stdin")))
	})

	t.Run("rejects a field without a value or a key", func(t *testing.T) {
		g := NewWithT(t)
		_, err := parseAPIFields([]rawField{{"tag", false}}, nil)
		g.Expect(err).To(MatchError(ContainSubstring("key=value")))

		_, err = parseAPIFields([]rawField{{"=x", false}}, nil)
		g.Expect(err).To(MatchError(ContainSubstring("key=value")))
	})
}

func TestAPIFieldsJSON(t *testing.T) {
	t.Run("nests bracketed keys and appends to [] arrays", func(t *testing.T) {
		g := NewWithT(t)
		got, err := apiFieldsJSON([]apiField{
			{"type", "birth"},
			{"date[from][year]", int64(1900)},
			{"date[from][month]", int64(5)},
			{"status[]", "undecided"},
			{"status[]", "confirmed"},
			{"isPrivate", false},
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(Equal(map[string]any{
			"type":      "birth",
			"date":      map[string]any{"from": map[string]any{"year": int64(1900), "month": int64(5)}},
			"status":    []any{"undecided", "confirmed"},
			"isPrivate": false,
		}))
	})

	t.Run("a key cannot be both a value and an object", func(t *testing.T) {
		g := NewWithT(t)
		_, err := apiFieldsJSON([]apiField{{"date", "x"}, {"date[from]", "y"}})
		g.Expect(err).To(MatchError(ContainSubstring("date")))
	})

	t.Run("a key cannot be given twice", func(t *testing.T) {
		g := NewWithT(t)
		_, err := apiFieldsJSON([]apiField{{"a", "1"}, {"a", "2"}})
		g.Expect(err).To(MatchError(ContainSubstring("more than once")))
	})

	t.Run("rejects malformed keys", func(t *testing.T) {
		g := NewWithT(t)
		for _, key := range []string{"a[b", "a]b", "[a]", "a[][b]", "a[b]c"} {
			_, err := apiFieldsJSON([]apiField{{key, "v"}})
			g.Expect(err).To(MatchError(ContainSubstring("invalid field key")), key)
		}
	})
}

func TestAPIFieldValues(t *testing.T) {
	g := NewWithT(t)
	got := apiFieldValues([]apiField{
		{"text", "Тюжин"}, {"page", int64(2)}, {"x", nil}, {"operation[]", "create"}, {"operation[]", "update"},
	})

	// Bracketed keys stay literal: PHP nests them on familio's side.
	g.Expect(got.Encode()).To(Equal(
		"operation%5B%5D=create&operation%5B%5D=update&page=2&text=%D0%A2%D1%8E%D0%B6%D0%B8%D0%BD&x="))
}

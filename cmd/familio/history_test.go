package main

import (
	"testing"
	"time"

	familio "github.com/dmalch/go-familio"
	. "github.com/onsi/gomega"
)

func TestHistoryListFlags_Filter(t *testing.T) {
	g := NewWithT(t)
	h := historyListFlags{
		persons:    multiFlag{"p-1", "p-2"},
		authors:    multiFlag{"a-1"},
		operations: multiFlag{"update"},
		causes:     multiFlag{"user"},
		block:      "event",
		eventType:  "birth",
		text:       "Тюжин",
		from:       "2026-07-01",
		till:       "2026-07-14",
		page:       2,
		limit:      50,
		asc:        true,
	}

	f, err := h.filter()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(f.PersonIDs).To(Equal([]string{"p-1", "p-2"}))
	g.Expect(f.AuthorIDs).To(Equal([]string{"a-1"}))
	g.Expect(f.Operations).To(Equal([]string{"update"}))
	g.Expect(f.Causes).To(Equal([]string{"user"}))
	g.Expect(f.DataTypes).To(Equal([]familio.HistoryDataType{{PersonDataBlock: "event", EventType: "birth"}}))
	g.Expect(f.Text).To(Equal("Тюжин"))
	g.Expect(f.Page).To(Equal(2))
	g.Expect(f.ItemsPerPage).To(Equal(50))
	g.Expect(f.Ascending).To(BeTrue())

	// Bare dates expand to the local day's bounds.
	g.Expect(f.From).To(Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)))
	g.Expect(f.Till).To(Equal(time.Date(2026, 7, 14, 23, 59, 59, 0, time.Local)))
}

func TestHistoryListFlags_TypeNarrowingNeedsBlock(t *testing.T) {
	g := NewWithT(t)
	h := historyListFlags{eventType: "birth"}
	_, err := h.filter()
	g.Expect(err).To(MatchError(ContainSubstring("-event-type/-source-type need -block")))
}

func TestParseHistoryDate(t *testing.T) {
	g := NewWithT(t)

	zero, err := parseHistoryDate("", false)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(zero.IsZero()).To(BeTrue())

	rfc, err := parseHistoryDate("2026-07-01T12:30:00+02:00", true)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rfc.Hour()).To(Equal(12)) // RFC3339 values pass through untouched

	_, err = parseHistoryDate("июль", false)
	g.Expect(err).To(MatchError(ContainSubstring("want YYYY-MM-DD or RFC3339")))
}

func TestBuildHistoryView_DecodesChanges(t *testing.T) {
	g := NewWithT(t)
	page := &familio.HistoryPage{
		Data: []familio.HistoryEntry{{
			Record: familio.HistoryRecord{
				ID:              1,
				Operation:       "update",
				PersonDataBlock: "biography",
				Changes:         []byte(`{"text":"Жил-был"}`),
			},
		}},
		Pager: familio.Pager{Page: 1, ItemsPerPage: 20, TotalItems: 1},
	}

	view := buildHistoryView(page)
	g.Expect(view.Pager.TotalItems).To(Equal(1))
	// The \u-escaped snapshot decodes into plain Cyrillic for display.
	g.Expect(view.Data[0].Record.Changes).To(Equal(map[string]any{"text": "Жил-был"}))
}

func TestRun_HistoryList_RejectsPositionals(t *testing.T) {
	g := NewWithT(t)
	code, _, errb := runArgs("history", "list", "extra")
	g.Expect(code).To(Equal(1))
	g.Expect(errb).To(ContainSubstring("takes no positional arguments"))
}

func TestRun_Help_ListsHistoryCommands(t *testing.T) {
	g := NewWithT(t)
	code, out, _ := runArgs("help")
	g.Expect(code).To(Equal(0))
	g.Expect(out).To(ContainSubstring("history list"))
	g.Expect(out).To(ContainSubstring("history filters"))
}

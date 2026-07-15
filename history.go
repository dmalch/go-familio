package familio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// historyPageSize is the default per-page size for the change-history list,
// matching the UI's smallest page option.
const historyPageSize = 20

// Pager is familio's paging envelope, shared by the paginated list endpoints.
type Pager struct {
	Page         int `json:"page"`
	ItemsPerPage int `json:"itemsPerPage"`
	TotalItems   int `json:"totalItems"`
}

// HistoryRecord is the change itself: what happened, when, and to which data
// block. Changes is the block's snapshot after the operation (for a delete,
// the state that was removed) and its shape follows PersonDataBlock — "basic"
// carries the flat basic fields, "event" an event-like object, "biography"
// {text}, "source" the source read shape — so it is kept as raw JSON. The API
// has no before/after diff; the UI derives it by comparing consecutive records.
type HistoryRecord struct {
	ID              int64           `json:"id"`
	HappenedAt      string          `json:"happenedAt"`
	Cause           string          `json:"cause"`
	Operation       string          `json:"operation"`
	PersonDataBlock string          `json:"personDataBlock"`
	Changes         json.RawMessage `json:"changes"`
}

// HistoryPersonRef identifies the person a history record belongs to.
type HistoryPersonRef struct {
	ID             string `json:"id"`
	LastName       string `json:"lastName"`
	FirstName      string `json:"firstName"`
	MiddleName     string `json:"middleName"`
	BirthLastName  string `json:"birthLastName"`
	BirthFirstName string `json:"birthFirstName"`
	Gender         string `json:"gender"`
}

// HistoryAuthor is who made the change. The zero uuid
// ("00000000-0000-0000-0000-000000000000") marks system-recorded changes
// (cause "initialization").
type HistoryAuthor struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// HistoryEntry is one row of the change-history list.
type HistoryEntry struct {
	Record HistoryRecord    `json:"record"`
	Person HistoryPersonRef `json:"person"`
	Author HistoryAuthor    `json:"author"`
}

// HistoryPage is one page of GET /api/v2/persons/history/<owner>.
type HistoryPage struct {
	Data  []HistoryEntry `json:"data"`
	Pager Pager          `json:"pager"`
}

// HistoryDataType is one personDataType[N] list filter: a data block
// ("basic", "event", "source", "biography"), optionally narrowed by the
// event-type key (block "event") or the source type — "register", "case",
// "catalog_person" (block "source").
type HistoryDataType struct {
	PersonDataBlock string
	EventType       string
	SourceType      string
}

// HistoryFilter narrows and pages the change-history list. The zero value
// requests the first page of everything, newest first.
type HistoryFilter struct {
	Text       string            // free-text search over the entries
	Operations []string          // "create", "update", "delete"
	Causes     []string          // "user", "initialization"
	AuthorIDs  []string          // editor user uuids; the zero uuid is the system author
	PersonIDs  []string          // limit to specific persons
	DataTypes  []HistoryDataType // limit to specific data blocks / event or source types
	From       time.Time         // happened-at range start (zero = unbounded)
	Till       time.Time         // happened-at range end (zero = unbounded)

	Page         int  // 1-based page number; 0 means 1
	ItemsPerPage int  // page size; 0 means historyPageSize
	Ascending    bool // oldest first instead of the default newest first
}

// query encodes the filter as the endpoint's query parameters. page,
// itemsPerPage, orderBy and orderDirection are all mandatory on the wire
// (the API answers 400 when any is missing), so they are always set.
func (f HistoryFilter) query() url.Values {
	page := max(f.Page, 1)
	perPage := f.ItemsPerPage
	if perPage < 1 {
		perPage = historyPageSize
	}
	direction := "desc"
	if f.Ascending {
		direction = "asc"
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("itemsPerPage", strconv.Itoa(perPage))
	q.Set("orderBy", "id")
	q.Set("orderDirection", direction)
	if f.Text != "" {
		q.Set("text", f.Text)
	}
	for _, v := range f.Operations {
		q.Add("operation[]", v)
	}
	for _, v := range f.Causes {
		q.Add("cause[]", v)
	}
	for _, v := range f.AuthorIDs {
		q.Add("authorId[]", v)
	}
	for _, v := range f.PersonIDs {
		q.Add("personId[]", v)
	}
	if !f.From.IsZero() {
		q.Set("date[from]", f.From.Format(time.RFC3339))
	}
	if !f.Till.IsZero() {
		q.Set("date[till]", f.Till.Format(time.RFC3339))
	}
	for i, dt := range f.DataTypes {
		prefix := fmt.Sprintf("personDataType[%d]", i)
		if dt.PersonDataBlock != "" {
			q.Set(prefix+"[personDataBlock]", dt.PersonDataBlock)
		}
		if dt.EventType != "" {
			q.Set(prefix+"[eventType]", dt.EventType)
		}
		if dt.SourceType != "" {
			q.Set(prefix+"[sourceType]", dt.SourceType)
		}
	}
	return q
}

// ListPersonsHistory fetches one page of the account's person change history
// («История изменений», a Familio Plus feature) via
// GET /api/v2/persons/history/<accountUuid>. The history lives under the
// authenticated account's uuid, so the owner is resolved from the session;
// page through by advancing HistoryFilter.Page until Pager.TotalItems is
// reached.
func (c *Client) ListPersonsHistory(ctx context.Context, filter HistoryFilter) (*HistoryPage, error) {
	owner, err := c.AccountUUID(ctx)
	if err != nil {
		return nil, err
	}

	req, err := c.newAuthedRequest(ctx, http.MethodGet, "persons/history/"+owner, filter.query(), nil)
	if err != nil {
		return nil, err
	}

	var page HistoryPage
	if err := c.do(req, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// HistoryFacet is one selectable value of a change-history filter facet,
// with the number of history entries carrying it.
type HistoryFacet struct {
	Item struct {
		Value        string `json:"value"`
		DisplayValue string `json:"displayValue"`
	} `json:"item"`
	Count int `json:"count"`
}

// HistoryDataTypeFacet is one value of the data-type facet; its value is the
// {personDataBlock, eventType, sourceType} triple used by
// HistoryFilter.DataTypes.
type HistoryDataTypeFacet struct {
	Item struct {
		Value struct {
			PersonDataBlock string  `json:"personDataBlock"`
			EventType       *string `json:"eventType"`
			SourceType      *string `json:"sourceType"`
		} `json:"value"`
		DisplayValue struct {
			Type    string  `json:"type"`
			Subtype *string `json:"subtype"`
		} `json:"displayValue"`
	} `json:"item"`
	Count int `json:"count"`
}

// HistoryFilters is the facet vocabulary of the change-history list: every
// author, operation, cause, person and data type present in the account's
// history, each with its entry count.
type HistoryFilters struct {
	Authors        []HistoryFacet         `json:"authorFilter"`
	Operations     []HistoryFacet         `json:"operationFilter"`
	Causes         []HistoryFacet         `json:"causeFilter"`
	Persons        []HistoryFacet         `json:"personFilter"`
	PersonsHasMore bool                   `json:"personFilterHasMore"`
	DataTypes      []HistoryDataTypeFacet `json:"personDataTypeFilter"`
}

// GetHistoryFilters fetches the change-history facet vocabularies via
// POST /api/v2/persons/history/<accountUuid>/get-filters-data. The counts are
// for the unfiltered history (the UI narrows them by POSTing the currently
// applied filters; this client always asks for the full vocabulary).
func (c *Client) GetHistoryFilters(ctx context.Context) (*HistoryFilters, error) {
	owner, err := c.AccountUUID(ctx)
	if err != nil {
		return nil, err
	}

	req, err := c.newAuthedRequest(ctx, http.MethodPost, "persons/history/"+owner+"/get-filters-data", nil, struct{}{})
	if err != nil {
		return nil, err
	}

	var filters HistoryFilters
	if err := c.do(req, &filters); err != nil {
		return nil, err
	}
	return &filters, nil
}

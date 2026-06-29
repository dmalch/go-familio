package familio

import (
	"context"
	"net/http"
	"net/url"
)

// Gender and privacy literals accepted by familio's /api/v2/persons surface.
const (
	GenderMale   = "male"
	GenderFemale = "female"

	PrivacyVisibleForAll = "visible_for_all"
	PrivacyInvisible     = "invisible"
)

// Event types and the participant roles seen in the tree editor.
const (
	EventBirth   = "birth"
	EventDeath   = "death"
	EventBaptism = "baptism" // христианское крещение (christening)
	EventWedding = "wedding"

	RoleChild  = "child"
	RoleParent = "parent"
	RoleOwner  = "owner"
	RoleSpouse = "spouse"

	// SelfRef is the placeholder personUuid for the person being created in the
	// same POST /api/v2/persons request (resolved server-side to the new uuid).
	SelfRef = "self"

	calendarGregorian = "gregorian"
	dateTypeEqual     = "equal"
	dateTypeBetween   = "between"
)

// BasicFields are the editable name/gender/privacy fields of a tree person —
// the "basic" object of POST /persons and the (flat) body of PUT /persons/<id>/basic.
type BasicFields struct {
	FirstName      string `json:"firstName"`
	LastName       string `json:"lastName"`
	MiddleName     string `json:"middleName"`     // отчество (patronymic)
	BirthFirstName string `json:"birthFirstName"` // maiden given name
	BirthLastName  string `json:"birthLastName"`  // maiden surname
	Gender         string `json:"gender"`
	Privacy        string `json:"privacy"`
}

// DatePart is one endpoint of a complex date. Year is required; Month/Day are
// optional (nil ⇒ unspecified). Type carries the per-part calendar.
type DatePart struct {
	Day   *int   `json:"day"`
	Month *int   `json:"month"`
	Year  int    `json:"year"`
	Type  string `json:"type"`
}

// EventDate is familio's complex-date object. A nil First means "date unknown".
type EventDate struct {
	Calendar string    `json:"calendar"`
	Type     string    `json:"type"`
	First    *DatePart `json:"first"`
	Second   *DatePart `json:"second"`
	// Formatted is server-computed on reads; never sent.
	Formatted string `json:"formatted,omitempty"`
}

// Participant links a person into an event by role.
type Participant struct {
	PersonUUID  string `json:"personUuid"`
	Role        string `json:"role"`
	DisplayName string `json:"displayName,omitempty"`
	Gender      string `json:"gender,omitempty"`
}

// Event is a life event (birth/death/wedding) with its participants.
type Event struct {
	UUID         *string       `json:"uuid"` // null on create; real uuid on read-back
	Type         string        `json:"type"`
	Date         EventDate     `json:"date"`
	Participants []Participant `json:"participants"`
	Settlement   *Settlement   `json:"settlement"` // place (Место); nil ⇒ null (no place)
	Comment      string        `json:"comment"`
	CreatedAt    string        `json:"createdAt,omitempty"`
	UpdatedAt    string        `json:"updatedAt,omitempty"`
}

// ID returns the event's uuid (empty when unset, e.g. a request-side event).
func (e *Event) ID() string {
	if e.UUID == nil {
		return ""
	}
	return *e.UUID
}

// BasicRecord is the basic person view. GET /persons/<uuid>/basic returns it
// flat (no displayName); the POST /persons 201 nests it under "basic" and does
// include displayName, so the field is populated on create but empty on a
// /basic read (use GetPersonDisplay for reads).
type BasicRecord struct {
	BasicFields
	UUID        string `json:"uuid"`
	DisplayName string `json:"displayName"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// personDisplay is the slice of the regularPerson view (GET /persons/<uuid>)
// the provider surfaces as the computed display_name.
type personDisplay struct {
	UUID        string `json:"uuid"`
	DisplayName string `json:"displayName"`
}

// personCreateBody is the POST /persons request envelope.
type personCreateBody struct {
	Basic     BasicFields `json:"basic"`
	Photo     *string     `json:"photo"`
	Events    []Event     `json:"events"`
	Biography *string     `json:"biography"`
}

// createResponse is the 201 body of POST /persons.
type createResponse struct {
	Basic  BasicRecord `json:"basic"`
	Events []Event     `json:"events"`
}

// CreatePersonInput is the caller-facing create payload: the basic fields plus
// the life events to attach. Exactly one birth event is required; build them
// with SelfBirthEvent / SelfDeathEvent. Biography, when non-nil, sets the
// person's initial biography text in the same request.
type CreatePersonInput struct {
	Basic     BasicFields
	Events    []Event
	Biography *string
}

// BirthEvent builds a birth event for childRef (SelfRef when the child is being
// created in the same POST /persons request, or the child's uuid when rebuilding
// an existing person's birth event), attaching 0–2 parent persons (role
// "parent"). familio upserts a person's single birth event by its child
// participant, so POSTing this event replaces the whole event (participants +
// date) in place — that is how parents are added/removed and the birth date is
// edited without recreating the person. place is the birth settlement uuid (""
// ⇒ no place) and comment is free text; like the date they are part of the event
// and must be re-sent on every upsert (a full replace would otherwise clear them).
func BirthEvent(date *DateRange, childRef string, parents []string, place, comment string) Event {
	parts := []Participant{{PersonUUID: childRef, Role: RoleChild}}
	for _, p := range parents {
		parts = append(parts, Participant{PersonUUID: p, Role: RoleParent})
	}
	return Event{Type: EventBirth, Date: EventDateFromRange(date), Participants: parts, Settlement: SettlementRef(place), Comment: comment}
}

// SelfBirthEvent builds the mandatory birth event for the person being created.
// Pass nil for an unknown date and "" for no place/comment.
func SelfBirthEvent(date *DateRange, place, comment string) Event {
	return BirthEvent(date, SelfRef, nil, place, comment)
}

// DeathEvent builds a death event owned by ownerRef (SelfRef on create, or the
// person's uuid when upserting an existing person's death event). Like the birth
// event, re-POSTing it replaces the person's single death event in place. place
// is the death settlement uuid ("" ⇒ no place); comment is free text.
func DeathEvent(date *DateRange, ownerRef, place, comment string) Event {
	return Event{
		Type:         EventDeath,
		Date:         EventDateFromRange(date),
		Participants: []Participant{{PersonUUID: ownerRef, Role: RoleOwner}},
		Settlement:   SettlementRef(place),
		Comment:      comment,
	}
}

// SelfDeathEvent builds an optional death event for the person being created.
func SelfDeathEvent(date *DateRange, place, comment string) Event {
	return DeathEvent(date, SelfRef, place, comment)
}

// BaptismEvent builds a christening (familio "baptism") event owned by ownerRef
// (SelfRef on create, or the person's uuid otherwise). Unlike birth/death, a
// baptism is a repeatable fact event: re-POSTing does NOT replace it, so editing
// a christening date means deleting the old event and creating a new one. place
// is the christening settlement uuid ("" ⇒ no place); comment is free text.
func BaptismEvent(date *DateRange, ownerRef, place, comment string) Event {
	return Event{
		Type:         EventBaptism,
		Date:         EventDateFromRange(date),
		Participants: []Participant{{PersonUUID: ownerRef, Role: RoleOwner}},
		Settlement:   SettlementRef(place),
		Comment:      comment,
	}
}

// SelfBaptismEvent builds an optional christening event for the person being created.
func SelfBaptismEvent(date *DateRange, place, comment string) Event {
	return BaptismEvent(date, SelfRef, place, comment)
}

// CreatePerson mints a new tree person (POST /api/v2/persons), owned by the
// authenticated account. Returns the server's basic record (incl. the new uuid)
// and the events it created.
func (c *Client) CreatePerson(ctx context.Context, in CreatePersonInput) (*createResponse, error) {
	// bearerToken also populates userUUID, used as ?owner=.
	if _, err := c.bearerToken(ctx); err != nil {
		return nil, err
	}
	query := url.Values{}
	if c.userUUID != "" {
		query.Set("owner", c.userUUID)
	}

	body := personCreateBody{Basic: in.Basic, Events: in.Events, Biography: in.Biography}
	req, err := c.newAuthedRequest(ctx, http.MethodPost, "persons", query, body)
	if err != nil {
		return nil, err
	}
	var resp createResponse
	if err := c.do(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetPersonBasic reads the editable basic fields (+ timestamps) of a person.
// ErrNotFound ⇒ the person is gone; the Read path drops it from state.
func (c *Client) GetPersonBasic(ctx context.Context, uuid string) (*BasicRecord, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodGet, "persons/"+uuid+"/basic", nil, nil)
	if err != nil {
		return nil, err
	}
	var rec BasicRecord
	if err := c.do(req, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// GetPersonEvents reads a person's life events (for birth/death dates).
func (c *Client) GetPersonEvents(ctx context.Context, uuid string) ([]Event, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodGet, "persons/"+uuid+"/events", nil, nil)
	if err != nil {
		return nil, err
	}
	var events []Event
	if err := c.do(req, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// GetPersonDisplay reads the computed display name from the regularPerson view.
func (c *Client) GetPersonDisplay(ctx context.Context, uuid string) (*personDisplay, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodGet, "persons/"+uuid, nil, nil)
	if err != nil {
		return nil, err
	}
	var d personDisplay
	if err := c.do(req, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// RegularRecord is the slice of the regularPerson view (GET /persons/<uuid>) the
// provider surfaces beyond the basic record: notably ownerId, the account that
// owns the profile, which is not present on the public settlement list.
type RegularRecord struct {
	UUID        string `json:"uuid"`
	DisplayName string `json:"displayName"`
	OwnerID     string `json:"ownerId"`
	Gender      string `json:"gender"`
	PrivacyType string `json:"privacyType"`
}

// GetPersonRegular reads the regularPerson view, including the owning account
// (ownerId) used to tell one's own tree from other researchers' profiles.
func (c *Client) GetPersonRegular(ctx context.Context, uuid string) (*RegularRecord, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodGet, "persons/"+uuid, nil, nil)
	if err != nil {
		return nil, err
	}
	var rec RegularRecord
	if err := c.do(req, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdatePersonBasic edits a person's basic fields. version is the optimistic-
// lock token (the updatedAt last read from GetPersonBasic), sent in the
// X-Base-Version header that familio's editor uses; a stale value is rejected
// with HTTP 409. Returns the refreshed basic record.
func (c *Client) UpdatePersonBasic(ctx context.Context, uuid string, fields BasicFields, version string) (*BasicRecord, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodPut, "persons/"+uuid+"/basic", nil, fields)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Base-Version", version)
	var rec BasicRecord
	if err := c.do(req, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// DeletePerson removes a tree person (DELETE /api/v2/persons/<uuid>).
func (c *Client) DeletePerson(ctx context.Context, uuid string) error {
	req, err := c.newAuthedRequest(ctx, http.MethodDelete, "persons/"+uuid, nil, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

package familio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Tag colours («Цвет»). familio stores a palette *code*, not a hex value — the
// UI maps the code to its pastel fill (see TagColorHex). Any other value is
// rejected by TagInput.Validate.
const (
	TagColorRoseMist     = "rose-mist"
	TagColorSoftPeach    = "soft-peach"
	TagColorLemonTint    = "lemon-tint"
	TagColorMintMist     = "mint-mist"
	TagColorIceBlue      = "ice-blue"
	TagColorLavenderHaze = "lavender-haze"
	TagColorLilacGlow    = "lilac-glow"
)

// Field limits the web editor enforces before it will submit a tag.
const (
	TagNameMaxLen        = 1000
	TagDescriptionMaxLen = 5000
)

// TagColors lists the accepted colour codes in the order the UI's picker shows
// them.
var TagColors = []string{
	TagColorRoseMist,
	TagColorSoftPeach,
	TagColorLemonTint,
	TagColorMintMist,
	TagColorIceBlue,
	TagColorLavenderHaze,
	TagColorLilacGlow,
}

// TagColorHex maps each colour code to the fill the web UI paints it with.
// familio never accepts or returns the hex itself; this is for callers that
// render tags themselves.
var TagColorHex = map[string]string{
	TagColorRoseMist:     "#FFEBEB",
	TagColorSoftPeach:    "#FFF4EB",
	TagColorLemonTint:    "#FDFFEB",
	TagColorMintMist:     "#EBFFEB",
	TagColorIceBlue:      "#EBFEFF",
	TagColorLavenderHaze: "#EBEBFF",
	TagColorLilacGlow:    "#FAEBFF",
}

// TagInput is the writable half of a tag («метка») — the body of both
// POST /tags and PUT /tags/<id>. Name travels on the wire as "tag".
type TagInput struct {
	Name        string `json:"tag"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// Tag is a tag as the API returns it. ID is the handle the update/delete and
// person assign/unassign calls take — and unlike every other familio resource a
// tag is keyed by a small **integer**, not a uuid. IsFree is server-computed:
// on a non-Plus account only the tags marked IsFree may be used, and only one of
// them, so an account without Familio Plus effectively gets a single working
// tag. The extra tags are still returned; the web UI renders them disabled.
// Nothing enforces that here.
type Tag struct {
	TagInput
	ID     int  `json:"id"`
	IsFree bool `json:"isFree"`
}

// Validate reports whether the input satisfies the rules the web editor
// applies before it submits: a non-blank name within TagNameMaxLen, one of
// TagColors, and a description within TagDescriptionMaxLen. It does not check
// the UI's case-insensitive uniqueness rule, which needs the account's whole
// tag list — compare against ListTags if you want it.
func (in TagInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("familio: tag name is required")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(in.Name)); n > TagNameMaxLen {
		return fmt.Errorf("familio: tag name is %d characters, over the %d limit", n, TagNameMaxLen)
	}
	if _, ok := TagColorHex[in.Color]; !ok {
		return fmt.Errorf("familio: invalid tag color %q (want one of: %s)", in.Color, strings.Join(TagColors, ", "))
	}
	if n := utf8.RuneCountInString(in.Description); n > TagDescriptionMaxLen {
		return fmt.Errorf("familio: tag description is %d characters, over the %d limit", n, TagDescriptionMaxLen)
	}
	return nil
}

// trimmed returns the input as the web editor sends it: the name trimmed and
// the description never null.
func (in TagInput) trimmed() TagInput {
	return TagInput{
		Name:        strings.TrimSpace(in.Name),
		Color:       in.Color,
		Description: in.Description,
	}
}

// PersonTags maps a person uuid to the tags assigned to that person — the
// response of GetTagsByPersons.
type PersonTags map[string][]Tag

// UnmarshalJSON decodes the map, tolerating the empty form the backend emits:
// an empty PHP associative array serializes as [] rather than {}, which the
// stock map decoder rejects.
func (p *PersonTags) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[]")) {
		*p = PersonTags{}
		return nil
	}
	var m map[string][]Tag
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*p = m
	return nil
}

// ListTags fetches the tags the authenticated account owns («Мои метки») via
// GET /api/v2/users/<accountUuid>/tags. The list is unpaged.
func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	owner, err := c.AccountUUID(ctx)
	if err != nil {
		return nil, err
	}

	req, err := c.newAuthedRequest(ctx, http.MethodGet, "users/"+owner+"/tags", nil, nil)
	if err != nil {
		return nil, err
	}

	var tags []Tag
	if err := c.do(req, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// CreateTag mints a new tag via POST /api/v2/tags and returns it, including the
// server-assigned ID. The input is validated locally first.
func (c *Client) CreateTag(ctx context.Context, in TagInput) (*Tag, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	req, err := c.newAuthedRequest(ctx, http.MethodPost, "tags", nil, in.trimmed())
	if err != nil {
		return nil, err
	}

	var tag Tag
	if err := c.do(req, &tag); err != nil {
		return nil, err
	}
	return &tag, nil
}

// UpdateTag replaces a tag's name, colour and description via
// PUT /api/v2/tags/<id> and returns the refreshed tag. All three fields are
// sent on every call, so a field left blank in the input is cleared. There is
// no optimistic-lock header on this endpoint.
func (c *Client) UpdateTag(ctx context.Context, id int, in TagInput) (*Tag, error) {
	if id <= 0 {
		return nil, errors.New("familio: no tag id given")
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}

	req, err := c.newAuthedRequest(ctx, http.MethodPut, "tags/"+strconv.Itoa(id), nil, in.trimmed())
	if err != nil {
		return nil, err
	}

	var tag Tag
	if err := c.do(req, &tag); err != nil {
		return nil, err
	}
	return &tag, nil
}

// DeleteTag removes a tag via DELETE /api/v2/tags/<id>, unassigning it from
// every person along the way.
func (c *Client) DeleteTag(ctx context.Context, id int) error {
	if id <= 0 {
		return errors.New("familio: no tag id given")
	}

	req, err := c.newAuthedRequest(ctx, http.MethodDelete, "tags/"+strconv.Itoa(id), nil, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// GetPersonTags reads the tags assigned to one person via
// GET /api/v2/persons/<personUuid>/tags. Only the person's author may manage
// its tags, so this is ErrAccessDenied on someone else's profile — and on a
// person that does not exist, which familio answers the same way.
func (c *Client) GetPersonTags(ctx context.Context, personUUID string) ([]Tag, error) {
	if personUUID == "" {
		return nil, errors.New("familio: no person uuid given")
	}

	req, err := c.newAuthedRequest(ctx, http.MethodGet, "persons/"+personUUID+"/tags", nil, nil)
	if err != nil {
		return nil, err
	}

	var tags []Tag
	if err := c.do(req, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// AssignPersonTags attaches tags to a person via
// POST /api/v2/persons/<personUuid>/tags and returns the person's refreshed tag
// list, which the endpoint echoes back. tagIDs are Tag.ID values. The call
// **adds** — it does not replace the person's set — and re-assigning an
// already-assigned tag is a no-op.
func (c *Client) AssignPersonTags(ctx context.Context, personUUID string, tagIDs []int) ([]Tag, error) {
	req, err := c.personTagsRequest(ctx, http.MethodPost, personUUID, tagIDs)
	if err != nil {
		return nil, err
	}

	var tags []Tag
	if err := c.do(req, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// UnassignPersonTags detaches tags from a person via
// DELETE /api/v2/persons/<personUuid>/tags — a DELETE that carries its tag ids
// as a JSON body. Unlike the assign call it answers 204 with no body, so read
// the result back with GetPersonTags if you need it. The tags themselves
// survive; use DeleteTag to remove one.
func (c *Client) UnassignPersonTags(ctx context.Context, personUUID string, tagIDs []int) error {
	req, err := c.personTagsRequest(ctx, http.MethodDelete, personUUID, tagIDs)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// personTagsRequest builds an assign/unassign request against a person's tags
// sub-resource. The body is a bare JSON array of tag ids, not an object.
func (c *Client) personTagsRequest(ctx context.Context, method, personUUID string, tagIDs []int) (*http.Request, error) {
	if personUUID == "" {
		return nil, errors.New("familio: no person uuid given")
	}
	if len(tagIDs) == 0 {
		return nil, errors.New("familio: no tag ids given")
	}
	return c.newAuthedRequest(ctx, method, "persons/"+personUUID+"/tags", nil, tagIDs)
}

// GetTagsByPersons reads the tags of several persons at once via
// POST /api/v2/tags/get-by-persons-id-list, the call the tree and person-list
// views use to paint their tag pills. Persons with no tags may be absent from
// the result rather than mapped to an empty slice.
func (c *Client) GetTagsByPersons(ctx context.Context, personUUIDs []string) (PersonTags, error) {
	if len(personUUIDs) == 0 {
		return nil, errors.New("familio: no person uuids given")
	}

	req, err := c.newAuthedRequest(ctx, http.MethodPost, "tags/get-by-persons-id-list", nil, personUUIDs)
	if err != nil {
		return nil, err
	}

	var tags PersonTags
	if err := c.do(req, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

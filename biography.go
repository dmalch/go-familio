package familio

import (
	"context"
	"net/http"
)

// Biography is a person's free-text life description (the familio "tab=2"
// panel). It is its own sub-resource carrying its own optimistic-lock version:
// UpdatedAt is the token to send back in X-Base-Version on the next edit, and
// is distinct from the basic record's updatedAt.
type Biography struct {
	Text      string `json:"text"`
	UpdatedAt string `json:"updatedAt"`
}

// biographyBody is the PUT /biography request envelope: only the text.
type biographyBody struct {
	Text string `json:"text"`
}

// GetPersonBiography reads a person's biography sub-resource. The returned
// UpdatedAt is the version to pass back to UpdatePersonBiography.
func (c *Client) GetPersonBiography(ctx context.Context, uuid string) (*Biography, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodGet, "persons/"+uuid+"/biography", nil, nil)
	if err != nil {
		return nil, err
	}
	var bio Biography
	if err := c.do(req, &bio); err != nil {
		return nil, err
	}
	return &bio, nil
}

// UpdatePersonBiography sets a person's biography text in place. version is the
// optimistic-lock token (the biography's own updatedAt, last read from
// GetPersonBiography), sent in the X-Base-Version header; a stale value is
// rejected with HTTP 409. Returns the refreshed biography (bumped UpdatedAt).
func (c *Client) UpdatePersonBiography(ctx context.Context, uuid, text, version string) (*Biography, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodPut, "persons/"+uuid+"/biography", nil, biographyBody{Text: text})
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Base-Version", version)
	var bio Biography
	if err := c.do(req, &bio); err != nil {
		return nil, err
	}
	return &bio, nil
}

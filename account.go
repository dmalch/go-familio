package familio

import (
	"context"
	"net/http"
)

// AccountUser is the account itself: its uuid (the same value as the JWT `uuid`
// claim and AccountUUID) and the address it logs in with.
type AccountUser struct {
	UUID  string `json:"uuid"`
	Email string `json:"email"`
}

// AccountDetails is the account holder's own name and gender — the fields
// familio renders as the signed-in user's identity.
type AccountDetails struct {
	DisplayName string `json:"displayName"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	MiddleName  string `json:"middleName"`
	Gender      string `json:"gender"`
}

// Profile is the authenticated account, as GET /api/v2/profile returns it: the
// login-level User and the display-level Details (the response's "profile" key).
type Profile struct {
	User    AccountUser    `json:"user"`
	Details AccountDetails `json:"profile"`
}

// GetProfile reads the authenticated account (GET /api/v2/profile): its uuid and
// email, plus the account holder's name and gender. Use it when the account's
// identity matters beyond the uuid AccountUUID returns — the uuid alone comes
// from the JWT and costs no request.
func (c *Client) GetProfile(ctx context.Context) (*Profile, error) {
	req, err := c.newAuthedRequest(ctx, http.MethodGet, "profile", nil, nil)
	if err != nil {
		return nil, err
	}
	var profile Profile
	if err := c.do(req, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

// AccountUUID returns the uuid of the authenticated account, read from the
// `uuid` claim of the scraped JWT (the same value sent as ?owner= on creates).
// It triggers a token scrape when one has not happened yet, so it doubles as a
// credential check: a missing or expired session surfaces as ErrNotLoggedIn.
func (c *Client) AccountUUID(ctx context.Context) (string, error) {
	if _, err := c.bearerToken(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userUUID, nil
}

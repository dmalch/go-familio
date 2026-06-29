package familio

import "context"

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

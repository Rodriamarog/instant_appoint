// Package social connects a clinic's Facebook Page — and the Instagram
// professional account linked to it — so the assistant answers Messenger and
// Instagram messages the way it answers WhatsApp.
//
// Both are connected with one Facebook Login. The browser sends the
// authorization code the login popup produced; everything from there happens
// here, because the exchange needs the app secret:
//
//  1. the code becomes a user token, and that becomes a long-lived one, so
//     the Page tokens read with it never expire;
//  2. the Pages the person granted are listed, each with its linked
//     Instagram account;
//  3. the chosen Page is subscribed to this app's webhooks, which is what
//     makes Meta deliver its messages (and its Instagram account's);
//  4. one social_accounts row is stored per channel.
//
// A person who manages several Pages chooses one, so step 3 onward runs on a
// second request. The Pages and their tokens are held in memory in between,
// for a few minutes, keyed by the person — the code cannot be exchanged twice
// and the tokens must not travel to the browser.
package social

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// The two channels a Page connects, as the channel column spells them.
const (
	Messenger = "messenger"
	Instagram = "instagram"
)

var (
	// ErrNoPages means the login granted no Page this app can answer from.
	ErrNoPages = errors.New("social: the login granted no Facebook Page")

	// ErrChoiceExpired means a Page was chosen with nothing pending to choose
	// from: the list expired, or the server restarted in between.
	ErrChoiceExpired = errors.New("social: there is no pending list of Pages to choose from")

	// ErrUnknownPage means the chosen Page is not one the login granted.
	ErrUnknownPage = errors.New("social: the chosen Page was not among the ones granted")

	// ErrTaken means another clinic already has this Page or Instagram
	// account connected.
	ErrTaken = errors.New("social: another account already has this Page or Instagram account connected")
)

// pendingTTL is how long a person has to choose a Page.
const pendingTTL = 10 * time.Minute

// Connector runs the connection flow. One per process, because it holds the
// Pages waiting to be chosen.
type Connector struct {
	app    core.App
	client *meta.Client
	now    func() time.Time

	mu      sync.Mutex
	pending map[string]pendingPages
}

type pendingPages struct {
	pages   []meta.Page
	expires time.Time
}

// NewConnector returns a connector over app and client.
func NewConnector(app core.App, client *meta.Client) *Connector {
	return &Connector{app: app, client: client, now: time.Now, pending: map[string]pendingPages{}}
}

// PageChoice is a Page offered to the person, without its token.
type PageChoice struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	InstagramUsername string `json:"instagram_username,omitempty"`
}

// Connected is what was connected.
type Connected struct {
	PageName          string `json:"page_name"`
	InstagramUsername string `json:"instagram_username,omitempty"`

	// HasInstagram is false when the Page has no linked Instagram
	// professional account, which the page explains how to fix.
	HasInstagram bool `json:"has_instagram"`
}

// Start exchanges a login code and either connects the person's only Page,
// returning what was connected, or returns the Pages to choose from.
func (c *Connector) Start(ctx context.Context, userID, code string) (*Connected, []PageChoice, error) {
	shortLived, err := c.client.ExchangeCode(ctx, code)
	if err != nil {
		return nil, nil, fmt.Errorf("exchange the login code: %w", err)
	}

	longLived, err := c.client.LongLivedUserToken(ctx, shortLived)
	if err != nil {
		return nil, nil, fmt.Errorf("extend the user token: %w", err)
	}

	pages, err := c.client.Pages(ctx, longLived)
	if err != nil {
		return nil, nil, fmt.Errorf("list the granted Pages: %w", err)
	}

	switch len(pages) {
	case 0:
		return nil, nil, ErrNoPages
	case 1:
		connected, err := c.connect(ctx, userID, pages[0])
		return connected, nil, err
	}

	c.mu.Lock()
	c.pending[userID] = pendingPages{pages: pages, expires: c.now().Add(pendingTTL)}
	c.mu.Unlock()

	choices := make([]PageChoice, 0, len(pages))
	for _, page := range pages {
		choice := PageChoice{ID: page.ID, Name: page.Name}
		if page.Instagram != nil {
			choice.InstagramUsername = page.Instagram.Username
		}
		choices = append(choices, choice)
	}

	return nil, choices, nil
}

// Choose connects one of the Pages Start offered.
func (c *Connector) Choose(ctx context.Context, userID, pageID string) (*Connected, error) {
	c.mu.Lock()
	pending, ok := c.pending[userID]
	if ok && c.now().After(pending.expires) {
		delete(c.pending, userID)
		ok = false
	}
	c.mu.Unlock()

	if !ok {
		return nil, ErrChoiceExpired
	}

	for _, page := range pending.pages {
		if page.ID != pageID {
			continue
		}

		connected, err := c.connect(ctx, userID, page)
		if err == nil {
			c.mu.Lock()
			delete(c.pending, userID)
			c.mu.Unlock()
		}

		return connected, err
	}

	return nil, ErrUnknownPage
}

// connect subscribes the Page and stores its rows, replacing whatever Page
// the clinic had before.
func (c *Connector) connect(ctx context.Context, userID string, page meta.Page) (*Connected, error) {
	// Checked before subscribing, so a refused connection changes nothing
	// at Meta either.
	if err := c.refuseIfTaken(userID, Messenger, page.ID); err != nil {
		return nil, err
	}
	if page.Instagram != nil {
		if err := c.refuseIfTaken(userID, Instagram, page.Instagram.ID); err != nil {
			return nil, err
		}
	}

	// Not advisory, unlike the WhatsApp signup's: a Page that is not
	// subscribed receives nothing, and here nothing is lost by failing —
	// the person logs in again.
	if err := c.client.SubscribePage(ctx, page.ID, page.AccessToken, meta.PageMessagingFields); err != nil {
		return nil, fmt.Errorf("subscribe the Page to webhooks: %w", err)
	}

	previous := c.pagesOf(userID)

	err := c.app.RunInTransaction(func(tx core.App) error {
		if err := upsert(tx, userID, Messenger, page.ID, page.ID, page.Name, "", page.AccessToken); err != nil {
			return err
		}

		if page.Instagram == nil {
			// The clinic's Instagram belonged to the Page it had before.
			return deleteRow(tx, userID, Instagram)
		}

		name := page.Instagram.Name
		if name == "" {
			name = page.Instagram.Username
		}

		return upsert(tx, userID, Instagram, page.Instagram.ID, page.ID, name, page.Instagram.Username, page.AccessToken)
	})
	if err != nil {
		return nil, fmt.Errorf("store the connected Page: %w", err)
	}

	// The Page this replaced keeps sending its messages here until it is
	// unsubscribed. Best effort: its token may be long revoked, and a stray
	// delivery for a Page no clinic has is dropped anyway.
	for pageID, token := range previous {
		if pageID != page.ID {
			c.unsubscribe(ctx, pageID, token)
		}
	}

	connected := &Connected{PageName: page.Name, HasInstagram: page.Instagram != nil}
	if page.Instagram != nil {
		connected.InstagramUsername = page.Instagram.Username
	}

	return connected, nil
}

// Disconnect removes one channel. The Page stays subscribed while the other
// channel still uses it — Instagram's messages arrive through the Page's
// subscription.
func (c *Connector) Disconnect(ctx context.Context, userID, channel string) error {
	if channel != Messenger && channel != Instagram {
		return fmt.Errorf("social: unknown channel %q", channel)
	}

	row, err := findRow(c.app, userID, channel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	pageID, token := row.GetString("page_id"), row.GetString("access_token")
	if err := c.app.Delete(row); err != nil {
		return err
	}

	if _, stillUsed := c.pagesOf(userID)[pageID]; !stillUsed {
		c.unsubscribe(ctx, pageID, token)
	}

	return nil
}

func (c *Connector) unsubscribe(ctx context.Context, pageID, token string) {
	if err := c.client.UnsubscribePage(ctx, pageID, token); err != nil {
		c.app.Logger().Warn("could not unsubscribe a Page that is no longer connected",
			"page_id", pageID, "error", meta.Sanitise(err))
	}
}

// pagesOf returns the Pages the clinic's rows send through, with a token for
// each.
func (c *Connector) pagesOf(userID string) map[string]string {
	rows, err := c.app.FindRecordsByFilter(migrations.SocialAccountsCollection,
		"user_id = {:userID}", "", 0, 0, dbx.Params{"userID": userID})
	if err != nil {
		return nil
	}

	pages := map[string]string{}
	for _, row := range rows {
		pages[row.GetString("page_id")] = row.GetString("access_token")
	}

	return pages
}

func (c *Connector) refuseIfTaken(userID, channel, externalID string) error {
	row, err := c.app.FindFirstRecordByFilter(migrations.SocialAccountsCollection,
		"channel = {:channel} && external_id = {:id} && user_id != {:userID}",
		dbx.Params{"channel": channel, "id": externalID, "userID": userID})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return err
	case row != nil:
		return ErrTaken
	}

	return nil
}

func findRow(app core.App, userID, channel string) (*core.Record, error) {
	return app.FindFirstRecordByFilter(migrations.SocialAccountsCollection,
		"user_id = {:userID} && channel = {:channel}",
		dbx.Params{"userID": userID, "channel": channel})
}

func upsert(app core.App, userID, channel, externalID, pageID, name, username, token string) error {
	row, err := findRow(app, userID, channel)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		collection, err := app.FindCollectionByNameOrId(migrations.SocialAccountsCollection)
		if err != nil {
			return err
		}
		row = core.NewRecord(collection)
		row.Set("user_id", userID)
		row.Set("channel", channel)
	case err != nil:
		return err
	}

	row.Set("external_id", externalID)
	row.Set("page_id", pageID)
	row.Set("name", name)
	row.Set("username", username)
	row.Set("access_token", token)

	return app.Save(row)
}

func deleteRow(app core.App, userID, channel string) error {
	row, err := findRow(app, userID, channel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	return app.Delete(row)
}

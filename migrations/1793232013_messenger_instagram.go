package migrations

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// SocialAccountsCollection holds one row per connected Facebook Page
// (Messenger) or Instagram professional account, with the Page access token
// that answers on its behalf. The id is pinned for the same reason
// whatsapp_accounts' is.
const (
	SocialAccountsCollection = "social_accounts"
	socialAccountsID         = "pbc_social_accounts"
)

// ChannelField is the column on conversations and whatsapp_messages saying
// which app the customer is writing from.
//
// The three words it holds are spelled out in internal/webhook, which may not
// import this package; DefaultChannel is the one this package needs, for the
// rows that predate the column — every one of them came in over WhatsApp.
const (
	ChannelField   = "channel"
	DefaultChannel = "whatsapp"
)

func init() {
	m.Register(func(app core.App) error {
		if err := app.Save(socialAccounts()); err != nil {
			return err
		}

		if err := addChannel(app, conversationsID, []string{
			// The customer key now includes the channel. A PSID and a phone
			// number cannot realistically collide, but a customer writing on
			// both Messenger and WhatsApp is two threads in two apps, and
			// replying to one from the other is not something Meta allows.
			"CREATE UNIQUE INDEX `idx_conversations_customer` ON `conversations` (`user_id`, `channel`, `customer_phone`, `status`)",
			"CREATE INDEX `idx_conversations_recent` ON `conversations` (`user_id`, `last_message_at`)",
		}); err != nil {
			return err
		}

		return addChannel(app, whatsAppMessagesID, nil)
	}, func(app core.App) error {
		if accounts, err := app.FindCollectionByNameOrId(SocialAccountsCollection); err == nil {
			if err := app.Delete(accounts); err != nil {
				return err
			}
		}

		conversations, err := app.FindCollectionByNameOrId(conversationsID)
		if err == nil {
			conversations.Fields.RemoveByName(ChannelField)
			conversations.Indexes = types.JSONArray[string]{
				"CREATE UNIQUE INDEX `idx_conversations_customer` ON `conversations` (`user_id`, `customer_phone`, `status`)",
				"CREATE INDEX `idx_conversations_recent` ON `conversations` (`user_id`, `last_message_at`)",
			}
			if err := app.Save(conversations); err != nil {
				return err
			}
		}

		messages, err := app.FindCollectionByNameOrId(whatsAppMessagesID)
		if err == nil {
			messages.Fields.RemoveByName(ChannelField)
			if err := app.Save(messages); err != nil {
				return err
			}
		}

		return nil
	})
}

// socialAccounts is the Messenger and Instagram counterpart of
// whatsapp_accounts.
//
// One Facebook Login yields both: a Page, and the Instagram professional
// account linked to it if there is one. They are separate rows because they
// are separate inboxes — a webhook names the Page id for Messenger and the
// Instagram account id for Instagram — but they share the Page's token, since
// Instagram messages are sent through the Page.
func socialAccounts() *core.Collection {
	collection := core.NewBaseCollection(SocialAccountsCollection, socialAccountsID)

	collection.Fields.Add(
		&core.TextField{Name: "user_id", Required: true, Max: 255},

		// "messenger" | "instagram".
		&core.TextField{Name: "channel", Required: true, Max: 20},

		// The id a webhook delivery is addressed to: the Page id for
		// Messenger, the Instagram account id for Instagram.
		&core.TextField{Name: "external_id", Required: true, Max: 255},

		// The Page messages are sent from. Equal to external_id for
		// Messenger; the linked Page for Instagram.
		&core.TextField{Name: "page_id", Required: true, Max: 255},

		// What the page shows: the Page's name, or the Instagram account's.
		&core.TextField{Name: "name", Max: 255},

		// The Instagram @handle, without the @. Empty for Messenger.
		&core.TextField{Name: "username", Max: 255},

		// A Page access token obtained from a long-lived user token, which
		// Meta issues without an expiry. Hidden, like every token in this
		// schema: Go reads it, no API response carries it.
		&core.TextField{Name: "access_token", Hidden: true, Max: 1000},

		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)

	// Owner-read, server-write, like whatsapp_accounts: every write carries a
	// token and happens in a Go handler.
	owner := "user_id = @request.auth.id"
	collection.ListRule = types.Pointer(owner)
	collection.ViewRule = types.Pointer(owner)
	collection.CreateRule = nil
	collection.UpdateRule = nil
	collection.DeleteRule = nil

	collection.Indexes = types.JSONArray[string]{
		// One clinic per Page or Instagram account. Two clinics holding the
		// same one would split its customers' messages between them by
		// whichever row a lookup happened to find first.
		"CREATE UNIQUE INDEX `idx_social_accounts_external` ON `social_accounts` (`channel`, `external_id`)",
		// One of each per clinic: connecting another Page replaces it.
		"CREATE UNIQUE INDEX `idx_social_accounts_user` ON `social_accounts` (`user_id`, `channel`)",
	}

	return collection
}

// addChannel adds the channel column to a collection, backfills it to
// whatsapp for the rows already there, and replaces the indexes when given
// any.
func addChannel(app core.App, id string, indexes []string) error {
	collection, err := app.FindCollectionByNameOrId(id)
	if err != nil {
		return nil // already gone
	}

	// The create-time default, so a writer that predates this column — the
	// reminder job writes rows here — keeps writing WhatsApp rows.
	collection.Fields.Add(&core.TextField{
		Name:                ChannelField,
		Max:                 20,
		AutogeneratePattern: DefaultChannel,
	})

	if indexes != nil {
		collection.Indexes = types.JSONArray[string](indexes)
	}

	if err := app.Save(collection); err != nil {
		return err
	}

	backfill := fmt.Sprintf(
		"UPDATE {{%s}} SET [[%s]] = {:channel} WHERE [[%s]] = '' OR [[%s]] IS NULL",
		collection.Name, ChannelField, ChannelField, ChannelField,
	)

	_, err = app.DB().NewQuery(backfill).Bind(dbx.Params{"channel": DefaultChannel}).Execute()

	return err
}

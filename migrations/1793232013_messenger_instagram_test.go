package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/migrations"
)

const theMessengerInstagramMigration = "1793232013_messenger_instagram.go"

// Every conversation and message that predates the column came in over
// WhatsApp, and the webhook now looks conversations up by channel: one left
// at "" would never be found again, and the customer's next message would
// start a second thread.
func TestConversationsAndMessagesFromBeforeTheColumnAreWhatsApp(t *testing.T) {
	app := newAppBefore(t, theMessengerInstagramMigration)

	conversation := saveUnder(t, app, migrations.ConversationsCollection, map[string]any{
		"user_id": "clinic-1", "customer_phone": "5215511111111", "status": "active",
	})
	message := saveUnder(t, app, migrations.WhatsAppMessagesCollection, map[string]any{
		"user_id": "clinic-1", "to_number": "5215511111111", "conversation_id": conversation,
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	for collection, id := range map[string]string{
		migrations.ConversationsCollection:    conversation,
		migrations.WhatsAppMessagesCollection: message,
	} {
		record, err := app.FindRecordById(collection, id)
		if err != nil {
			t.Fatalf("%s row is gone: %v", collection, err)
		}
		if got := record.GetString(migrations.ChannelField); got != migrations.DefaultChannel {
			t.Errorf("%s.channel = %q, want %q", collection, got, migrations.DefaultChannel)
		}
	}
}

// The same customer key on two channels is two conversations, and one
// channel still allows only one active conversation per customer.
func TestTheActiveConversationIsUniquePerChannel(t *testing.T) {
	app := newAppBefore(t, theMessengerInstagramMigration)
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	saveUnder(t, app, migrations.ConversationsCollection, map[string]any{
		"user_id": "clinic-1", "customer_phone": "123", "status": "active", "channel": "whatsapp",
	})
	saveUnder(t, app, migrations.ConversationsCollection, map[string]any{
		"user_id": "clinic-1", "customer_phone": "123", "status": "active", "channel": "messenger",
	})

	collection, err := app.FindCollectionByNameOrId(migrations.ConversationsCollection)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := newRecord(collection, map[string]any{
		"user_id": "clinic-1", "customer_phone": "123", "status": "active", "channel": "messenger",
	})
	if err := app.Save(duplicate); err == nil {
		t.Error("a second active Messenger conversation with the same customer was accepted")
	}
}

func TestOnePageCannotBeConnectedToTwoClinics(t *testing.T) {
	app := newAppBefore(t, theMessengerInstagramMigration)
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	saveUnder(t, app, migrations.SocialAccountsCollection, map[string]any{
		"user_id": "clinic-1", "channel": "messenger", "external_id": "P1", "page_id": "P1",
	})

	collection, err := app.FindCollectionByNameOrId(migrations.SocialAccountsCollection)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Save(newRecord(collection, map[string]any{
		"user_id": "clinic-2", "channel": "messenger", "external_id": "P1", "page_id": "P1",
	})); err == nil {
		t.Error("a second clinic was given the same Page")
	}
}

func newRecord(collection *core.Collection, values map[string]any) *core.Record {
	record := core.NewRecord(collection)
	for key, value := range values {
		record.Set(key, value)
	}

	return record
}

package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// The collection names and their pinned ids. The ids match the ones the
// JavaScript migrations used, so a database carried over from the old stack
// and one built from these migrations name the same collections the same way.
const (
	ConversationsCollection = "conversations"
	conversationsID         = "pbc_conversations"

	WhatsAppMessagesCollection = "whatsapp_messages"
	whatsAppMessagesID         = "pbc_whatsapp_messages"
)

func init() {
	m.Register(func(app core.App) error {
		if err := app.Save(conversations()); err != nil {
			return err
		}

		return app.Save(whatsAppMessages())
	}, func(app core.App) error {
		for _, name := range []string{WhatsAppMessagesCollection, ConversationsCollection} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue // already gone
			}
			if err := app.Delete(collection); err != nil {
				return err
			}
		}

		return nil
	})
}

// conversations is one customer's thread with one business.
//
// It exists so the dashboard can show a list of customers rather than a flat
// stream of messages, and so the AI can be given the right history: every
// message carries the id of the conversation it belongs to.
func conversations() *core.Collection {
	collection := core.NewBaseCollection(ConversationsCollection, conversationsID)

	collection.Fields.Add(
		&core.TextField{Name: "user_id", Required: true, Max: 255},

		// Digits including the country code and without a leading +, which is
		// the form Meta sends and the form every lookup here compares against.
		&core.TextField{Name: "customer_phone", Required: true, Max: 50},

		// Only "active" is written today. Kept as free text rather than a
		// select because it is part of a unique index: a select would reject a
		// value a future close-conversation feature invents, and the index
		// would then be enforcing a rule nobody can satisfy.
		&core.TextField{Name: "status", Max: 50},

		// ISO-8601 as internal/isotime writes it. Text, not date: the whole
		// app sorts and range-filters these as strings, and the fixed-width
		// format is what makes that match chronological order.
		&core.TextField{Name: "last_message_at", Max: 50},

		// The business's Meta phone number id, carried on the conversation so
		// a human replying from the dashboard is sent from the same number the
		// customer has been talking to. Every delivery overwrites it, so a
		// conversation follows a number change.
		//
		// This field was declared in no JavaScript migration even though the
		// webhook writes it on both the create and the update path. A database
		// built from those migrations rejected every inbound message with an
		// "unknown field" error, which is why it is called out here.
		&core.TextField{Name: "phone_number_id", Max: 255},

		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)

	ownerReadServerWrite(collection)

	collection.Indexes = types.JSONArray[string]{
		// A customer has one active conversation. Two messages from one
		// customer arriving together would otherwise each create one, leaving
		// the dashboard showing that customer twice with half a conversation
		// each and the AI answering from half the history.
		//
		// status is part of the key rather than a predicate on it so that
		// closing a conversation frees the customer for a new one.
		"CREATE UNIQUE INDEX `idx_conversations_customer` ON `conversations` (`user_id`, `customer_phone`, `status`)",

		// The dashboard's conversation list: this user's threads, most
		// recently active first.
		"CREATE INDEX `idx_conversations_recent` ON `conversations` (`user_id`, `last_message_at`)",
	}

	return collection
}

// whatsAppMessages is every message this app has seen or sent: customer
// messages, AI replies, echoes of what the business typed in the WhatsApp
// Business app, and appointment reminders.
func whatsAppMessages() *core.Collection {
	collection := core.NewBaseCollection(WhatsAppMessagesCollection, whatsAppMessagesID)

	collection.Fields.Add(
		&core.TextField{Name: "user_id", Required: true, Max: 255},

		// to_number holds the customer either way: the sender of an inbound
		// message and the recipient of an outbound one. The name is from when
		// this collection only recorded reminders going out.
		&core.TextField{Name: "to_number", Required: true, Max: 50},

		// A body is rejected rather than truncated when it exceeds the
		// maximum, and rejecting an inbound message loses it. WhatsApp allows
		// 4096 characters today and Meta has raised that before, so this is
		// an order of magnitude of headroom — an unset Max is not, because
		// PocketBase reads that as its 5000-character default.
		&core.TextField{Name: "message_content", Max: 65535},

		// "text" for anything in a conversation, "reminder" for an appointment
		// reminder. Part of the reminder index's predicate.
		&core.TextField{Name: "message_type", Max: 50},

		// "inbound" | "outbound". What tells the AI whose turn a stored
		// message was.
		&core.TextField{Name: "direction", Max: 20},

		&core.TextField{Name: "conversation_id", Max: 255},

		// WhatsApp's own id for the message — a wamid for anything that went
		// through Meta, and empty for a row written by a path that never got
		// an id back. The deduplication key.
		&core.TextField{Name: "message_id", Max: 255},

		// The calendar event a reminder is about. Empty for conversation
		// messages. Part of the reminder index's key.
		&core.TextField{Name: "event_id", Max: 255},

		// "sent" | "failed", written by the reminder and send paths only; the
		// webhook leaves it empty. Part of the reminder index's predicate.
		&core.TextField{Name: "status", Max: 50},

		// When this app received or sent the message, not Meta's timestamp.
		// ISO-8601 text, as above, and the sort key for a conversation.
		&core.TextField{Name: "sent_at", Max: 50},

		// Which connected number carried the message. Declared in no
		// JavaScript migration even though the send path writes it, inside a
		// catch that only logged the failure — so the write had been failing
		// unnoticed.
		&core.TextField{Name: "whatsapp_account_id", Max: 255},

		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)

	ownerReadServerWrite(collection)

	collection.Indexes = types.JSONArray[string]{
		// Deduplication, enforced by the database rather than by the handler.
		// Two deliveries of one WhatsApp message can be in flight at once, and
		// a check-then-write in Go has a window between the two halves that
		// only the database can close. Without this the customer is answered
		// twice.
		//
		// Predicated on a non-empty id because rows with no wamid — a reminder
		// that failed before Meta answered, anything written by a path that
		// never sent — must not collide with each other.
		"CREATE UNIQUE INDEX `idx_whatsapp_messages_message_id` ON `whatsapp_messages` (`message_id`) " +
			"WHERE `message_id` != ''",

		// One reminder per appointment, ever. The row recording the send is
		// itself the lock, so a restart mid-run, or two servers racing the
		// same scheduled minute, cannot message a customer twice. The
		// predicate keeps it off the appointment's ordinary messages and
		// leaves a failed reminder retryable.
		"CREATE UNIQUE INDEX `idx_whatsapp_messages_reminder_sent` ON `whatsapp_messages` (`event_id`) " +
			"WHERE `message_type` = 'reminder' AND `status` = 'sent'",

		// Reading a conversation, oldest first: the query behind both the
		// dashboard thread and the history handed to the AI.
		"CREATE INDEX `idx_whatsapp_messages_conversation` ON `whatsapp_messages` (`conversation_id`, `sent_at`)",

		// The reminder page lists this user's sent reminders.
		"CREATE INDEX `idx_whatsapp_messages_user` ON `whatsapp_messages` (`user_id`, `sent_at`)",
	}

	return collection
}

// ownerReadServerWrite scopes reads to the owning user and closes writes to
// everyone but superusers and server-side Go.
//
// The SPA talks to PocketBase directly with the user's own token, so these
// rules are the only thing standing between one business and another's
// customer conversations. The schema this was recovered from left all five
// open, which let any signed-in user read and rewrite every other business's
// messages.
//
// The write rules are set to nil explicitly rather than left alone. They mean
// the same thing, but one of them says so.
func ownerReadServerWrite(collection *core.Collection) {
	owner := "user_id = @request.auth.id"

	collection.ListRule = types.Pointer(owner)
	collection.ViewRule = types.Pointer(owner)
	collection.CreateRule = nil
	collection.UpdateRule = nil
	collection.DeleteRule = nil
}

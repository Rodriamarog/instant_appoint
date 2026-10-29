// Package chat is the server side of the dashboard's WhatsApp inbox.
//
// Almost all of the inbox is the browser talking to PocketBase directly: the
// conversations and whatsapp_messages collections are owner-read, so the
// access rules are what keep one business's customers away from another's, and
// a handler in front of them would add nothing but a hop.
//
// Sending is the exception, and this package is only that exception. A reply
// goes out through Meta's Graph API with the connected account's access token,
// and that token must never reach a browser — the collection hides it, and
// even if it did not, handing it over would let any page send as the business.
// The same rules that protect the collections also close them to writes from
// the browser, so the outbound row is written here too.
package chat

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	"github.com/Rodriamarog/instant_appoint/internal/meta"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// SendPath is where the chat page posts a human reply.
const SendPath = "/api/chat/send"

// The two values every message in a conversation carries, as opposed to a
// reminder. They are what the AI history query and the reminder indexes read.
const (
	conversationMessage = "text"
	outbound            = "outbound"
)

// Register binds this package's routes onto r.
//
// Only a concrete method and path is registered: an all-method /api/...
// wildcard conflicts with the SPA's GET /{path...} catch-all, and the standard
// ServeMux answers that conflict with a panic at boot rather than a precedence
// rule. Registering one pattern twice panics for the same reason, hence the
// guard.
func Register(r *router.Router[*core.RequestEvent], client *meta.Client) {
	if r.HasRoute(http.MethodPost, SendPath) {
		return
	}

	r.POST(SendPath, send(client)).Bind(apis.RequireAuth("users"))
}

// sendRequest is what the chat page posts. The field names are the ones the
// page already sent, kept so the port is a swap rather than a rewrite.
type sendRequest struct {
	ConversationID string `json:"conversation_id"`
	Message        string `json:"message"`
}

// send delivers one typed reply to the customer and records it.
func send(client *meta.Client) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		var request sendRequest
		if err := e.BindBody(&request); err != nil {
			return e.BadRequestError("The message could not be read.", err)
		}

		if request.ConversationID == "" || request.Message == "" {
			return e.BadRequestError("A reply needs a conversation and something to say.", nil)
		}

		conversation, err := ownConversation(e, request.ConversationID)
		if err != nil {
			return err
		}

		account, err := accountForConversation(e, conversation)
		if err != nil {
			return err
		}

		customer := conversation.GetString("customer_phone")

		messageID, err := client.SendText(e.Request.Context(),
			account.GetString("phone_number_id"), account.GetString("access_token"),
			customer, request.Message)
		if err != nil {
			return metaError(e, err)
		}

		// One clock read for both writes, so the message and the thread it
		// moved agree on when the reply happened. isotime is the only
		// formatter allowed here: both columns are text and both are sorted
		// as strings, so a near-miss format sorts into the wrong place.
		at := isotime.Now()

		message, err := storeMessage(e, conversation, account, request.Message, messageID, at)
		if err != nil {
			// The customer has the message. Saying the send failed would be a
			// lie that invites a second send, so the failure is logged and
			// the request still succeeds — the thread is missing a row, which
			// a reload from Meta's side will not fix but a human can see.
			e.App.Logger().Error("a sent reply could not be stored",
				"conversation_id", conversation.Id, "message_id", messageID, "error", err)

			return e.InternalServerError("The message was sent but could not be saved to the conversation.", err)
		}

		touchConversation(e, conversation, at)

		// The whole row goes back, as the route this replaces returned it.
		// The page merges it into the thread by id, so the reply shows
		// immediately whether or not the live connection is working.
		return e.JSON(http.StatusOK, message)
	}
}

// ownConversation resolves the conversation the caller named, or the response
// to send when it is not theirs to reply in.
//
// A conversation belonging to someone else is reported as missing rather than
// as forbidden. The distinction would tell a caller which conversation ids
// exist, and PocketBase's own record API — which the same page reads the
// thread through — already answers another person's record with a 404, so this
// keeps one story.
func ownConversation(e *core.RequestEvent, id string) (*core.Record, error) {
	conversation, err := e.App.FindRecordById(migrations.ConversationsCollection, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, e.NotFoundError("That conversation does not exist.", err)

	case err != nil:
		e.App.Logger().Error("could not read the conversation", "conversation_id", id, "error", err)

		return nil, e.InternalServerError("Could not open that conversation.", err)
	}

	if conversation.GetString("user_id") != e.Auth.Id {
		return nil, e.NotFoundError("That conversation does not exist.", nil)
	}

	return conversation, nil
}

// accountForConversation resolves which connected number the reply goes out
// from.
//
// It is the conversation's phone_number_id, not the caller's account, and that
// is deliberate: a business with two connected numbers must answer each
// customer on the number that customer wrote to. Resolving by user_id instead
// would send half of them from the wrong number — which is why this lookup is
// not whatsapp.AccountFor, the by-user lookup the template routes use.
//
// Being by number, the lookup authorises nothing on its own; ownConversation
// above is what established that this caller may reply here at all.
//
// The filter is as loose as the webhook's — no account_type, no is_active.
// Tightening it would mean a number mid-reconnect silently refuses replies to
// customers whose messages the webhook is still accepting on it.
func accountForConversation(e *core.RequestEvent, conversation *core.Record) (*core.Record, error) {
	number := conversation.GetString("phone_number_id")
	if number == "" {
		// Written on every delivery, so an empty one means this conversation
		// predates the field or was made by hand.
		return nil, e.NotFoundError("This conversation is not linked to a WhatsApp number yet, so there is nothing to reply from.", nil)
	}

	account, err := e.App.FindFirstRecordByFilter(
		migrations.WhatsAppAccountsCollection,
		"phone_number_id = {:phoneNumberID}",
		dbx.Params{"phoneNumberID": number},
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, e.NotFoundError("The WhatsApp number this conversation arrived on is no longer connected.", err)

	case err != nil:
		e.App.Logger().Error("could not resolve the conversation's WhatsApp account",
			"phone_number_id", number, "error", err)

		return nil, e.InternalServerError("Could not read the WhatsApp number to reply from.", err)
	}

	return account, nil
}

// storeMessage writes the outbound row.
//
// The eight fields the webhook writes for an inbound message, plus the account
// that carried it. message_id is the wamid Meta returned; SendText errors
// rather than returning an empty one, so a stored row always has a real id and
// the deduplication index has something to work with.
func storeMessage(e *core.RequestEvent, conversation, account *core.Record, text, messageID, at string) (*core.Record, error) {
	collection, err := e.App.FindCollectionByNameOrId(migrations.WhatsAppMessagesCollection)
	if err != nil {
		return nil, err
	}

	message := core.NewRecord(collection)
	message.Set("user_id", conversation.GetString("user_id"))
	message.Set("to_number", conversation.GetString("customer_phone"))
	message.Set("message_content", text)
	message.Set("message_type", conversationMessage)
	message.Set("direction", outbound)
	message.Set("conversation_id", conversation.Id)
	message.Set("message_id", messageID)
	message.Set("sent_at", at)
	message.Set("whatsapp_account_id", account.Id)

	if err := e.App.Save(message); err != nil {
		return nil, err
	}

	return message, nil
}

// touchConversation moves the thread to the top of the list.
//
// The route this replaces did not do this, and the omission showed: the list
// is sorted by last_message_at, so a thread a human had just replied in sank
// below ones nobody had touched for days. The webhook already keeps the field
// current for everything arriving; this keeps it current for everything
// leaving.
//
// A failure here is logged and swallowed. The message is sent and stored; the
// only casualty is the ordering of a list that the next delivery will fix.
func touchConversation(e *core.RequestEvent, conversation *core.Record, at string) {
	conversation.Set("last_message_at", at)

	if err := e.App.Save(conversation); err != nil {
		e.App.Logger().Error("could not update the conversation's last activity",
			"conversation_id", conversation.Id, "error", err)
	}
}

// metaError turns a Graph failure into one readable sentence.
//
// Meta's raw body reaches neither the browser nor the log: a failing call can
// echo the app secret in the query string it quotes. meta.Explain and
// meta.Sanitise own that rule; see them for what is left out and why.
func metaError(e *core.RequestEvent, err error) error {
	e.App.Logger().Error("sending a chat reply failed", "user_id", e.Auth.Id, "error", meta.Sanitise(err))

	if explanation, ok := meta.Explain(err); ok {
		return e.InternalServerError("WhatsApp rejected the message: "+explanation, err)
	}

	return e.InternalServerError("Sending the message failed. Please try again.", err)
}

package chat_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Rodriamarog/instant_appoint/internal/isotime"
	"github.com/Rodriamarog/instant_appoint/migrations"
)

// What the send leaves behind, and what it does not.

// A refused send must not leave a row claiming the customer was told
// something. Meta's own sentence is worth repeating — it is what says to wait
// for the customer to write first — but nothing else from the response is.
func TestARefusedSendStoresNothingAndRepeatsOnlyMetasSentence(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	account(t, app, user.Id, "200000000000002", ownerToken)
	conv := conversation(t, app, user.Id, "5216641234567", "200000000000002")

	g := newGraph(t)
	g.status = http.StatusBadRequest
	g.reply = `{"error":{"message":"Message failed to send because more than 24 hours have passed.",` +
		`"type":"OAuthException","code":131047,"fbtrace_id":"Ax5","error_data":` +
		`{"details":"client_secret=` + ownerToken + `"}}}`

	mux := newMux(t, app, g.client(), user)

	rec := send(t, mux, `{"conversation_id":"`+conv.Id+`","message":"Still there?"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", rec.Code, rec.Body)
	}

	message, _ := decode(t, rec)["message"].(string)
	if !strings.Contains(message, "more than 24 hours have passed") {
		t.Errorf("message = %q, want it to quote Meta's explanation", message)
	}
	if strings.Contains(rec.Body.String(), ownerToken) || strings.Contains(rec.Body.String(), "fbtrace_id") {
		t.Errorf("the response echoes Meta's raw body: %s", rec.Body)
	}

	if stored := messagesIn(t, app, conv.Id); len(stored) != 0 {
		t.Errorf("a refused send stored %d messages", len(stored))
	}

	reloaded, err := app.FindRecordById(migrations.ConversationsCollection, conv.Id)
	if err != nil {
		t.Fatalf("reload the conversation: %v", err)
	}
	if got := reloaded.GetString("last_message_at"); got != "2026-10-29T12:00:00.000Z" {
		t.Errorf("last_message_at = %q, want it untouched by a send that never happened", got)
	}
}

// Replying is activity, so the thread moves to the top of the list the same
// way an arriving message moves it. The route this replaces left the field
// alone, and a thread a human had just answered sank below ones nobody had
// touched for days.
func TestAReplyMovesTheConversationToTheTopOfTheList(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	account(t, app, user.Id, "200000000000002", ownerToken)
	conv := conversation(t, app, user.Id, "5216641234567", "200000000000002")
	mux := newMux(t, app, newGraph(t).client(), user)

	rec := send(t, mux, `{"conversation_id":"`+conv.Id+`","message":"Booked you in."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	reloaded, err := app.FindRecordById(migrations.ConversationsCollection, conv.Id)
	if err != nil {
		t.Fatalf("reload the conversation: %v", err)
	}

	activity := reloaded.GetString("last_message_at")
	if activity == "2026-10-29T12:00:00.000Z" {
		t.Fatalf("last_message_at is still %q; the reply did not count as activity", activity)
	}

	stored := messagesIn(t, app, conv.Id)
	if len(stored) != 1 {
		t.Fatalf("stored %d messages, want 1", len(stored))
	}
	if sentAt := stored[0].GetString("sent_at"); sentAt != activity {
		t.Errorf("the message says %q and the conversation says %q; one clock read should serve both", sentAt, activity)
	}
}

// Both columns are text and both are sorted as strings, so a timestamp of a
// different width sorts into the wrong place and nothing complains. This is
// the one shape that sorts correctly.
func TestTheStoredTimestampsAreInTheFormatEverythingElseSortsBy(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	account(t, app, user.Id, "200000000000002", ownerToken)
	conv := conversation(t, app, user.Id, "5216641234567", "200000000000002")
	mux := newMux(t, app, newGraph(t).client(), user)

	if rec := send(t, mux, `{"conversation_id":"`+conv.Id+`","message":"Noted."}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	reloaded, err := app.FindRecordById(migrations.ConversationsCollection, conv.Id)
	if err != nil {
		t.Fatalf("reload the conversation: %v", err)
	}

	for field, value := range map[string]string{
		"whatsapp_messages.sent_at":     messagesIn(t, app, conv.Id)[0].GetString("sent_at"),
		"conversations.last_message_at": reloaded.GetString("last_message_at"),
	} {
		if len(value) != len(isotime.Layout) {
			t.Errorf("%s = %q, want the %d-character stored shape", field, value, len(isotime.Layout))
		}
		if _, err := isotime.Parse(value); err != nil {
			t.Errorf("%s = %q: %v", field, value, err)
		}
	}
}

// The reply is in the response as well as in the database, so the page can
// show it the moment it is sent rather than waiting for the live connection to
// echo it back.
func TestTheSentReplyComesBackInTheResponse(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	account(t, app, user.Id, "200000000000002", ownerToken)
	conv := conversation(t, app, user.Id, "5216641234567", "200000000000002")
	mux := newMux(t, app, newGraph(t).client(), user)

	rec := send(t, mux, `{"conversation_id":"`+conv.Id+`","message":"See you then."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	body := decode(t, rec)
	stored := messagesIn(t, app, conv.Id)
	if len(stored) != 1 {
		t.Fatalf("stored %d messages, want 1", len(stored))
	}

	for field, want := range map[string]any{
		"id":              stored[0].Id,
		"direction":       "outbound",
		"message_content": "See you then.",
		"conversation_id": conv.Id,
	} {
		if body[field] != want {
			t.Errorf("response %s = %v, want %v", field, body[field], want)
		}
	}
}

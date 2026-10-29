package chat_test

import (
	"net/http"
	"testing"
)

// Who may reply, and as whom. The route spends an account's access token and
// writes to a collection the browser cannot write to, so everything it refuses
// has to be refused here — there is no second line.

// The token is the whole point of the route existing, so an anonymous caller
// must not reach Meta at all.
func TestSendingRequiresASignedInUser(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	account(t, app, user.Id, "200000000000002", ownerToken)
	conv := conversation(t, app, user.Id, "5216641234567", "200000000000002")
	g := newGraph(t)
	mux := newMux(t, app, g.client(), nil)

	rec := send(t, mux, `{"conversation_id":"`+conv.Id+`","message":"Hello?"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (body: %s)", rec.Code, rec.Body)
	}
	if len(g.requests) != 0 {
		t.Errorf("an anonymous request reached Meta: %v", g.requests)
	}
	if stored := messagesIn(t, app, conv.Id); len(stored) != 0 {
		t.Errorf("an anonymous request stored %d messages", len(stored))
	}
}

// Someone else's conversation is not a conversation this caller can put words
// into. Reported as missing rather than as forbidden, so the route does not
// answer "does this id exist?" for a stranger.
func TestAPersonCannotReplyInSomeoneElsesConversation(t *testing.T) {
	app := newApp(t)
	owner := newUser(t, app, "owner@example.test")
	stranger := newUser(t, app, "stranger@example.test")
	account(t, app, owner.Id, "200000000000002", ownerToken)
	conv := conversation(t, app, owner.Id, "5216641234567", "200000000000002")
	g := newGraph(t)
	mux := newMux(t, app, g.client(), stranger)

	rec := send(t, mux, `{"conversation_id":"`+conv.Id+`","message":"Who is this?"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body)
	}
	if len(g.requests) != 0 {
		t.Errorf("a stranger's request reached Meta: %v", g.requests)
	}
	if stored := messagesIn(t, app, conv.Id); len(stored) != 0 {
		t.Errorf("a stranger's request stored %d messages in another person's conversation", len(stored))
	}
}

// A conversation id nobody owns reads the same as one somebody else owns.
func TestReplyingToAConversationThatDoesNotExistIsRefused(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	account(t, app, user.Id, "200000000000002", ownerToken)
	g := newGraph(t)
	mux := newMux(t, app, g.client(), user)

	rec := send(t, mux, `{"conversation_id":"no_such_conversation","message":"Hello?"}`)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", rec.Code, rec.Body)
	}
	if len(g.requests) != 0 {
		t.Errorf("a request for an unknown conversation reached Meta: %v", g.requests)
	}
}

// Both halves of the body are required, and neither failure costs a Graph
// call.
func TestAReplyNeedsAConversationAndSomethingToSay(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	account(t, app, user.Id, "200000000000002", ownerToken)
	conv := conversation(t, app, user.Id, "5216641234567", "200000000000002")
	g := newGraph(t)
	mux := newMux(t, app, g.client(), user)

	for name, body := range map[string]string{
		"no conversation": `{"message":"Hello?"}`,
		"no message":      `{"conversation_id":"` + conv.Id + `"}`,
		"empty message":   `{"conversation_id":"` + conv.Id + `","message":""}`,
		"not json":        `this is not json`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := send(t, mux, body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
			}
		})
	}

	if len(g.requests) != 0 {
		t.Errorf("an incomplete request reached Meta: %v", g.requests)
	}
}

// The number the conversation arrived on decides which account sends, so a
// business with two connected numbers answers each customer on their own
// number. Resolving the account from the caller's user id instead would send
// one of these two replies from the wrong number — which is exactly what this
// test exists to catch.
func TestTheReplyGoesOutFromTheNumberTheCustomerWroteTo(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "twonumbers@example.test")
	account(t, app, user.Id, "111111111111111", "EAAG-clinic-token")
	account(t, app, user.Id, "222222222222222", "EAAG-spa-token")

	clinic := conversation(t, app, user.Id, "5216641111111", "111111111111111")
	spa := conversation(t, app, user.Id, "5216642222222", "222222222222222")

	g := newGraph(t)
	mux := newMux(t, app, g.client(), user)

	if rec := send(t, mux, `{"conversation_id":"`+clinic.Id+`","message":"Clinic here."}`); rec.Code != http.StatusOK {
		t.Fatalf("clinic send: status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}
	if rec := send(t, mux, `{"conversation_id":"`+spa.Id+`","message":"Spa here."}`); rec.Code != http.StatusOK {
		t.Fatalf("spa send: status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	if len(g.requests) != 2 {
		t.Fatalf("made %d Graph calls, want 2: %v", len(g.requests), g.requests)
	}

	if got := g.requests[0]; got.path != "/v21.0/111111111111111/messages" || got.bearer != "EAAG-clinic-token" {
		t.Errorf("clinic reply went out as %q with %q, want the clinic number and its own token", got.path, got.bearer)
	}
	if got := g.requests[1]; got.path != "/v21.0/222222222222222/messages" || got.bearer != "EAAG-spa-token" {
		t.Errorf("spa reply went out as %q with %q, want the spa number and its own token", got.path, got.bearer)
	}
}

// A number that has been disconnected, or a conversation written before the
// field existed, leaves nothing to reply from. That is said rather than
// guessed at with the caller's other account.
func TestAConversationWithNoReachableNumberIsRefused(t *testing.T) {
	app := newApp(t)
	user := newUser(t, app, "owner@example.test")
	account(t, app, user.Id, "200000000000002", ownerToken)

	// Each case needs its own customer: one customer has one active
	// conversation, and the collection's unique index enforces it.
	for _, unreachable := range []struct{ name, customer, number string }{
		{"no number at all", "5216647777777", ""},
		{"a number nobody holds", "5216648888888", "999999999999999"},
	} {
		t.Run(unreachable.name, func(t *testing.T) {
			conv := conversation(t, app, user.Id, unreachable.customer, unreachable.number)
			g := newGraph(t)
			mux := newMux(t, app, g.client(), user)

			rec := send(t, mux, `{"conversation_id":"`+conv.Id+`","message":"Hello?"}`)

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404 (body: %s)", rec.Code, rec.Body)
			}
			if len(g.requests) != 0 {
				t.Errorf("the request reached Meta anyway: %v", g.requests)
			}
		})
	}
}

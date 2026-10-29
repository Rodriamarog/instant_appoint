package pbstore_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"github.com/Rodriamarog/instant_appoint/internal/pbstore"
	"github.com/Rodriamarog/instant_appoint/internal/webhook"
	_ "github.com/Rodriamarog/instant_appoint/migrations"
)

// newApp returns a bootstrapped app with every migration applied, backed by a
// throwaway data directory.
func newApp(t *testing.T) core.App {
	t.Helper()

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  t.TempDir(),
		HideStartBanner: true,
	})

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() {
		if err := app.ResetBootstrapState(); err != nil {
			t.Errorf("ResetBootstrapState: %v", err)
		}
	})

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	return app
}

// connectAccount stores a connected WhatsApp number and returns its user id.
func connectAccount(t *testing.T, app core.App, phoneNumberID string, extra map[string]any) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("whatsapp_accounts")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	userID := "user" + fmt.Sprintf("%011d", len(phoneNumberID)*7919%1e11)
	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	record.Set("phone_number_id", phoneNumberID)
	record.Set("access_token", "token-for-"+phoneNumberID)
	record.Set("account_type", "business_api")
	record.Set("is_active", true)
	record.Set("status", "connected")
	for name, value := range extra {
		record.Set(name, value)
	}

	if err := app.Save(record); err != nil {
		t.Fatalf("store the account: %v", err)
	}

	return userID
}

func TestFindAccountResolvesAConnectedNumber(t *testing.T) {
	app := newApp(t)
	userID := connectAccount(t, app, "555000111", nil)
	store := pbstore.NewWebhookStore(app)

	account, err := store.FindAccount(context.Background(), "555000111")
	if err != nil {
		t.Fatalf("FindAccount: %v", err)
	}
	if account == nil {
		t.Fatal("no account was found for a connected number")
	}
	if account.UserID != userID {
		t.Errorf("UserID = %q, want %q", account.UserID, userID)
	}
	if account.AccessToken != "token-for-555000111" {
		t.Errorf("AccessToken = %q, want the stored token", account.AccessToken)
	}
}

// The router forwards every number it serves, most of which belong to other
// apps behind it. An unknown one is the ordinary case, not an error: treating
// it as one would answer the router with a 500 and have it redeliver somebody
// else's traffic forever.
func TestFindAccountReturnsNothingForAnUnknownNumber(t *testing.T) {
	app := newApp(t)
	connectAccount(t, app, "555000111", nil)
	store := pbstore.NewWebhookStore(app)

	account, err := store.FindAccount(context.Background(), "1078215982040649")
	if err != nil {
		t.Fatalf("FindAccount: %v", err)
	}
	if account != nil {
		t.Errorf("found %+v for a number no account has", account)
	}
}

// Deliberately looser than every other account lookup in this app, which also
// require account_type and is_active. A message that arrived is a message that
// has to be stored: a number mid-reconnect, or one still carrying the retired
// QR flow's account_type, would otherwise have its customers' messages dropped
// on the floor.
func TestFindAccountDoesNotFilterOnTheAccountsState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra map[string]any
	}{
		{"an inactive account", map[string]any{"is_active": false, "status": "disconnected"}},
		{"an account from the retired QR flow", map[string]any{"account_type": "web"}},
		{"an account with no account_type", map[string]any{"account_type": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newApp(t)
			connectAccount(t, app, "555000111", tc.extra)
			store := pbstore.NewWebhookStore(app)

			account, err := store.FindAccount(context.Background(), "555000111")
			if err != nil {
				t.Fatalf("FindAccount: %v", err)
			}
			if account == nil {
				t.Error("the message would have been dropped: no account was found")
			}
		})
	}
}

func TestHasMessage(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)
	ctx := context.Background()

	if err := store.SaveMessage(ctx, "user000000000001", webhook.StoredMessage{
		MessageID: "wamid.1", ConversationID: "conv1", CustomerPhone: "5215511111111",
		Content: "hola", Direction: webhook.Inbound, SentAt: "2026-10-13T12:00:00.000Z",
	}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	for _, tc := range []struct {
		id   string
		want bool
	}{
		{"wamid.1", true},
		{"wamid.2", false},
	} {
		got, err := store.HasMessage(ctx, tc.id)
		if err != nil {
			t.Fatalf("HasMessage(%q): %v", tc.id, err)
		}
		if got != tc.want {
			t.Errorf("HasMessage(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// Every column the dashboard and the AI read has to be populated, under the
// name they read it by. A typo here stores a message that nothing can find.
func TestSaveMessageWritesEveryColumnTheAppReads(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)

	err := store.SaveMessage(context.Background(), "user000000000001", webhook.StoredMessage{
		MessageID:      "wamid.1",
		ConversationID: "conv1",
		CustomerPhone:  "5215511111111",
		Content:        "hola",
		Direction:      webhook.Inbound,
		SentAt:         "2026-10-13T12:00:00.000Z",
	})
	if err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}

	record, err := app.FindFirstRecordByFilter("whatsapp_messages", "message_id = {:id}", dbx.Params{"id": "wamid.1"})
	if err != nil {
		t.Fatalf("read the stored message back: %v", err)
	}

	for _, tc := range []struct{ column, want string }{
		{"user_id", "user000000000001"},
		{"to_number", "5215511111111"},
		{"message_content", "hola"},
		{"message_type", "text"},
		{"direction", "inbound"},
		{"conversation_id", "conv1"},
		{"sent_at", "2026-10-13T12:00:00.000Z"},
		{"message_id", "wamid.1"},
	} {
		if got := record.GetString(tc.column); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.column, got, tc.want)
		}
	}
}

// The handler's in-flight guard is per process. A redelivery landing on a
// second server, or after a restart, reaches the store with the same id, and
// the store has to recognise it rather than fail the whole delivery — a
// failure would answer the router non-2xx and have it try again forever.
func TestSaveMessageIgnoresAMessageItAlreadyHas(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)
	ctx := context.Background()

	message := webhook.StoredMessage{
		MessageID: "wamid.1", ConversationID: "conv1", CustomerPhone: "5215511111111",
		Content: "hola", Direction: webhook.Inbound, SentAt: "2026-10-13T12:00:00.000Z",
	}
	if err := store.SaveMessage(ctx, "user000000000001", message); err != nil {
		t.Fatalf("the first write: %v", err)
	}
	if err := store.SaveMessage(ctx, "user000000000001", message); err != nil {
		t.Errorf("the second write failed instead of being ignored: %v", err)
	}

	count, err := app.CountRecords("whatsapp_messages")
	if err != nil {
		t.Fatalf("CountRecords: %v", err)
	}
	if count != 1 {
		t.Errorf("stored %d messages, want 1", count)
	}
}

// A message with no WhatsApp id cannot be recognised on redelivery, so it must
// not be silently swallowed by the deduplication path either.
func TestSaveMessageStoresEveryMessageWithNoID(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		err := store.SaveMessage(ctx, "user000000000001", webhook.StoredMessage{
			ConversationID: "conv1", CustomerPhone: "5215511111111",
			Content: "hola", Direction: webhook.Outbound, SentAt: "2026-10-13T12:00:00.000Z",
		})
		if err != nil {
			t.Fatalf("write %d: %v", i+1, err)
		}
	}

	count, err := app.CountRecords("whatsapp_messages")
	if err != nil {
		t.Fatalf("CountRecords: %v", err)
	}
	if count != 2 {
		t.Errorf("stored %d messages, want 2", count)
	}
}

func TestUpsertConversationCreatesThenReuses(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)
	ctx := context.Background()

	first, err := store.UpsertConversation(ctx, "user000000000001", "5215511111111", "555000111", "2026-10-13T12:00:00.000Z")
	if err != nil {
		t.Fatalf("the first upsert: %v", err)
	}
	if first == "" {
		t.Fatal("no conversation id was returned")
	}

	second, err := store.UpsertConversation(ctx, "user000000000001", "5215511111111", "555000222", "2026-10-13T12:05:00.000Z")
	if err != nil {
		t.Fatalf("the second upsert: %v", err)
	}
	if second != first {
		t.Errorf("a second message from the same customer opened conversation %q instead of reusing %q", second, first)
	}

	record, err := app.FindRecordById("conversations", first)
	if err != nil {
		t.Fatalf("read the conversation back: %v", err)
	}
	for _, tc := range []struct{ column, want string }{
		{"user_id", "user000000000001"},
		{"customer_phone", "5215511111111"},
		{"status", "active"},
		{"last_message_at", "2026-10-13T12:05:00.000Z"},
		// The number the customer is talking to now, so a human replying from
		// the dashboard is sent from it.
		{"phone_number_id", "555000222"},
	} {
		if got := record.GetString(tc.column); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.column, got, tc.want)
		}
	}
}

func TestUpsertConversationSeparatesCustomersAndBusinesses(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)
	ctx := context.Background()

	mine, err := store.UpsertConversation(ctx, "user000000000001", "5215511111111", "555000111", "2026-10-13T12:00:00.000Z")
	if err != nil {
		t.Fatalf("UpsertConversation: %v", err)
	}

	for _, tc := range []struct{ name, userID, phone string }{
		{"another customer of the same business", "user000000000001", "5215522222222"},
		{"the same customer of another business", "user000000000002", "5215511111111"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other, err := store.UpsertConversation(ctx, tc.userID, tc.phone, "555000111", "2026-10-13T12:00:00.000Z")
			if err != nil {
				t.Fatalf("UpsertConversation: %v", err)
			}
			if other == mine {
				t.Error("two different threads were merged into one conversation")
			}
		})
	}
}

// Two messages from one customer arriving together both find no conversation
// and both try to create one. Only the database sees both, and the loser has
// to recover to the winner's conversation rather than fail the delivery.
func TestConcurrentUpsertsSettleOnOneConversation(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)

	const attempts = 8
	ids := make([]string, attempts)
	errs := make([]error, attempts)

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	for i := 0; i < attempts; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			ids[i], errs[i] = store.UpsertConversation(
				context.Background(), "user000000000001", "5215511111111", "555000111", "2026-10-13T12:00:00.000Z")
		}()
	}
	start.Done()
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("upsert %d failed: %v", i, err)
		}
	}
	for i, id := range ids {
		if id != ids[0] {
			t.Errorf("upsert %d returned conversation %q, want the single conversation %q", i, id, ids[0])
		}
	}

	count, err := app.CountRecords("conversations")
	if err != nil {
		t.Fatalf("CountRecords: %v", err)
	}
	if count != 1 {
		t.Errorf("stored %d conversations, want 1", count)
	}
}

// The AI is told the conversation oldest first. Out of order it reads the
// exchange backwards and answers questions that have already been answered.
func TestConversationHistoryIsOldestFirst(t *testing.T) {
	app := newApp(t)
	store := pbstore.NewWebhookStore(app)
	ctx := context.Background()

	// Written newest first, so insertion order cannot be mistaken for sorting.
	for _, m := range []webhook.StoredMessage{
		{MessageID: "wamid.3", ConversationID: "conv1", Content: "el martes", Direction: webhook.Inbound, SentAt: "2026-10-13T12:02:00.000Z"},
		{MessageID: "wamid.1", ConversationID: "conv1", Content: "hola", Direction: webhook.Inbound, SentAt: "2026-10-13T12:00:00.000Z"},
		{MessageID: "wamid.other", ConversationID: "conv2", Content: "otra", Direction: webhook.Inbound, SentAt: "2026-10-13T12:01:00.000Z"},
		{MessageID: "wamid.2", ConversationID: "conv1", Content: "¿cuándo?", Direction: webhook.Outbound, SentAt: "2026-10-13T12:01:00.000Z"},
	} {
		m.CustomerPhone = "5215511111111"
		if err := store.SaveMessage(ctx, "user000000000001", m); err != nil {
			t.Fatalf("SaveMessage %s: %v", m.MessageID, err)
		}
	}

	history, err := store.ConversationHistory(ctx, "conv1")
	if err != nil {
		t.Fatalf("ConversationHistory: %v", err)
	}

	var got []string
	for _, m := range history {
		got = append(got, m.MessageID)
	}
	want := []string{"wamid.1", "wamid.2", "wamid.3"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("history = %v, want %v", got, want)
	}

	if len(history) > 0 {
		first := history[0]
		if first.Content != "hola" || first.Direction != webhook.Inbound || first.CustomerPhone != "5215511111111" {
			t.Errorf("the oldest message came back as %+v", first)
		}
	}
}

// --- the unique-violation predicate --------------------------------------

// The predicate has to cover two different error shapes, and the obvious
// implementation only covers one of them. app.Save runs the driver error
// through PocketBase's normaliser, which replaces it with an ozzo validation
// error and discards the original — so errors.As against the sqlite error type
// returns false on exactly the path production uses. An errors.As-only
// predicate would never fire, and every redelivered message would turn a
// harmless no-op into a failed delivery.
func TestIsUniqueViolationRecognisesTheSavePath(t *testing.T) {
	app := newApp(t)

	collection, err := app.FindCollectionByNameOrId("whatsapp_messages")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	write := func() error {
		record := core.NewRecord(collection)
		record.Set("user_id", "user000000000001")
		record.Set("to_number", "5215511111111")
		record.Set("message_id", "wamid.1")

		return app.Save(record)
	}

	if err := write(); err != nil {
		t.Fatalf("the first save: %v", err)
	}

	err = write()
	if err == nil {
		t.Fatal("the second save succeeded; the unique index is not in force")
	}
	if !pbstore.IsUniqueViolation(err) {
		t.Errorf("IsUniqueViolation did not recognise an app.Save duplicate: %#v", err)
	}
}

// The other shape: a raw database write returns the driver's own error, code
// 2067, with nothing in between to rewrite it.
func TestIsUniqueViolationRecognisesTheRawPath(t *testing.T) {
	app := newApp(t)

	if _, err := app.DB().NewQuery("CREATE TABLE raw_unique (a TEXT UNIQUE)").Execute(); err != nil {
		t.Fatalf("create the table: %v", err)
	}
	if _, err := app.DB().NewQuery("INSERT INTO raw_unique (a) VALUES ('x')").Execute(); err != nil {
		t.Fatalf("the first insert: %v", err)
	}

	_, err := app.DB().NewQuery("INSERT INTO raw_unique (a) VALUES ('x')").Execute()
	if err == nil {
		t.Fatal("the second insert succeeded")
	}
	if !pbstore.IsUniqueViolation(err) {
		t.Errorf("IsUniqueViolation did not recognise a driver duplicate: %#v", err)
	}
}

func TestIsUniqueViolationIgnoresEverythingElse(t *testing.T) {
	app := newApp(t)

	collection, err := app.FindCollectionByNameOrId("whatsapp_messages")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	// A required field left empty: a validation error of the same Go type as
	// a unique violation, which is what makes the distinction worth a test.
	missingRequired := app.Save(core.NewRecord(collection))
	if missingRequired == nil {
		t.Fatal("a record with no user_id was accepted")
	}

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"no error", nil},
		{"an unrelated error", errors.New("PocketBase down")},
		{"a wrapped unrelated error", fmt.Errorf("store the message: %w", errors.New("timeout"))},
		{"another validation error", missingRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if pbstore.IsUniqueViolation(tc.err) {
				t.Errorf("IsUniqueViolation(%v) = true, want false", tc.err)
			}
		})
	}
}

// A wrapped unique violation must still be recognised: the error travels up
// through layers that add context to it.
func TestIsUniqueViolationSeesThroughWrapping(t *testing.T) {
	app := newApp(t)

	collection, err := app.FindCollectionByNameOrId("conversations")
	if err != nil {
		t.Fatalf("FindCollectionByNameOrId: %v", err)
	}

	write := func() error {
		record := core.NewRecord(collection)
		record.Set("user_id", "user000000000001")
		record.Set("customer_phone", "5215511111111")
		record.Set("status", "active")

		return app.Save(record)
	}
	if err := write(); err != nil {
		t.Fatalf("the first save: %v", err)
	}

	duplicate := fmt.Errorf("upsert the conversation: %w", write())
	if !pbstore.IsUniqueViolation(duplicate) {
		t.Errorf("IsUniqueViolation did not see through the wrapping: %#v", duplicate)
	}
}

// The store satisfies the interface the handler is written against, which is
// the whole reason the handler can be tested with no database.
var _ webhook.Store = (*pbstore.WebhookStore)(nil)

package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// WhatsAppAccountsCollection is the name and the pinned id of the collection
// holding one row per connected WhatsApp number. The id is pinned so a
// relation declared in another migration resolves to the same collection in
// every environment.
const (
	WhatsAppAccountsCollection = "whatsapp_accounts"
	whatsAppAccountsID         = "pbc_3975332653"
)

func init() {
	m.Register(func(app core.App) error {
		accounts := core.NewBaseCollection(WhatsAppAccountsCollection, whatsAppAccountsID)

		accounts.Fields.Add(
			&core.TextField{Name: "user_id", Required: true, Max: 255},

			// The three secrets. Hidden affects serialization only:
			// record.GetString reads them in Go, but they never reach a REST
			// response, a realtime payload or a client-side filter. A client
			// that filters or sorts on one gets a hard 400, so the SPA must
			// not mention them in a query.
			&core.TextField{Name: "access_token", Hidden: true, Max: 1000},
			&core.TextField{Name: "qr_code", Hidden: true, Max: 100000},
			&core.TextField{Name: "session_id", Hidden: true, Max: 255},

			// "connected" | "disconnected" | "initializing" | "waiting_qr".
			// Kept as free text rather than a select: the legacy QR service
			// writes values this app never reads, and a select would reject
			// them on a row it does not own.
			&core.TextField{Name: "status", Max: 50},

			// Required on a bool means "must be true" in PocketBase, which
			// would make a disconnected account unstorable. Left optional.
			&core.BoolField{Name: "is_active"},

			&core.TextField{Name: "account_type", Max: 50},
			&core.TextField{Name: "waba_id", Max: 255},
			&core.TextField{Name: "phone_number_id", Max: 255},

			// Meta's display name for the number, fetched during the token
			// exchange. Declared in none of the legacy JS migrations even
			// though the exchange writes it, which made the whole exchange
			// fail with "unknown field" on a database built from them.
			&core.TextField{Name: "verified_name", Max: 255},

			// Written by the legacy QR service only; the Cloud API path never
			// sets it, so the page falls back to phone_number_id.
			&core.TextField{Name: "phone_number", Max: 50},

			&core.TextField{Name: "disconnect_reason", Max: 500},

			// ISO-8601 string, like every other timestamp in this schema.
			// Also undeclared in the legacy migrations, which left
			// disconnected accounts stuck at connected/is_active forever.
			&core.TextField{Name: "disconnected_at", Max: 50},

			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)

		// Reads are owner-scoped. Writes are nil, meaning superusers and
		// server-side Go only: every write to this collection carries a Meta
		// access token and happens in a Go handler, which bypasses rules.
		owner := "user_id = @request.auth.id"
		accounts.ListRule = types.Pointer(owner)
		accounts.ViewRule = types.Pointer(owner)
		accounts.CreateRule = nil
		accounts.UpdateRule = nil
		accounts.DeleteRule = nil

		accounts.Indexes = types.JSONArray[string]{
			// The webhook resolves an account by phone_number_id once per
			// inbound message; this is the hottest lookup in the app.
			"CREATE INDEX `idx_whatsapp_accounts_phone_number_id` ON `whatsapp_accounts` (`phone_number_id`)",
			// Every page and handler that resolves "this user's connected
			// business account" starts from user_id.
			"CREATE INDEX `idx_whatsapp_accounts_user_id` ON `whatsapp_accounts` (`user_id`, `account_type`, `is_active`)",
		}

		return app.Save(accounts)
	}, func(app core.App) error {
		accounts, err := app.FindCollectionByNameOrId(WhatsAppAccountsCollection)
		if err != nil {
			return nil // already gone
		}

		return app.Delete(accounts)
	})
}

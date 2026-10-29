// Package isotime is the single place this app turns an instant into the
// timestamp string it stores.
//
// Every application timestamp — calendar_events.start_time and .end_time,
// whatsapp_messages.sent_at, conversations.last_message_at — lives in a
// PocketBase *text* column, and every ordering and range query over them is a
// byte-by-byte comparison performed by SQLite. That only matches chronological
// order while every writer emits the identical fixed-width shape:
//
//	YYYY-MM-DDTHH:mm:ss.sssZ
//
// which is what JavaScript's Date.prototype.toISOString() produces and what
// web/src/lib/iso-time.ts keeps producing on the browser side.
//
// The near misses are what make this a package rather than a one-liner at each
// call site:
//
//   - time.RFC3339 emits no fraction at all. 'Z' is 0x5A and '.' is 0x2E, so
//     "…:00Z" sorts after "…:00.999Z" — a whole second lands after every
//     millisecond inside it.
//   - time.RFC3339Nano trims trailing zeros, so ".5Z" sorts after ".45Z".
//   - A non-UTC location with a "Z07:00" layout emits "+00:00"-style offsets.
//     '+' is 0x2B, which sorts before every digit and before '.', breaking
//     >= / <= range filters outright.
package isotime

import (
	"fmt"
	"time"
)

// Layout is the one timestamp layout this app writes.
//
// ".000" is Go's fixed-width-with-trailing-zeros fraction, unlike ".999"
// which trims. The Z is a literal in the layout, not "Z07:00", so it is only
// ever correct for a UTC time — which is why Format converts first.
const Layout = "2006-01-02T15:04:05.000Z"

// Format renders t as the stored timestamp string: UTC, millisecond
// precision, 24 characters, always ending in a literal Z.
func Format(t time.Time) string {
	return t.UTC().Format(Layout)
}

// Now is Format applied to the current instant.
func Now() string {
	return Format(time.Now())
}

// Parse reads a stored timestamp back. It is deliberately strict: a value
// that is valid ISO-8601 but a different width — no fraction, a trimmed
// fraction, a numeric offset — is rejected rather than quietly accepted,
// because such a value is already sorting wrongly wherever it is stored.
func Parse(value string) (time.Time, error) {
	t, err := time.Parse(Layout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("timestamp %q is not in the stored format %q: %w", value, Layout, err)
	}

	return t.UTC(), nil
}

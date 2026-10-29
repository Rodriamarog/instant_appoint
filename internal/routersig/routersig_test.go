package routersig_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/routersig"
)

const secret = "test-router-secret"

// now is the frozen clock the legacy vitest suite uses, so a failure here and a
// failure there describe the same instant.
var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// sign reproduces what the Neurocrow router does, independently of the code
// under test: hex HMAC-SHA256 over "<timestamp>.<rawBody>".
func sign(key string, timestamp string, body string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(timestamp + "." + body))

	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// headers builds a delivery's headers. An empty value omits the header, which
// is how a bare POST with no router headers at all is expressed.
func headers(signature, timestamp string) http.Header {
	h := http.Header{}
	if signature != "" {
		h.Set(routersig.SignatureHeader, signature)
	}
	if timestamp != "" {
		h.Set(routersig.TimestampHeader, timestamp)
	}
	h.Set(routersig.DeliveryIDHeader, "d-1")

	return h
}

func TestValid(t *testing.T) {
	body := `{"entry":[]}`
	nowSec := strconv.FormatInt(now.Unix(), 10)

	for _, tc := range []struct {
		name      string
		secret    string
		body      string
		signature string
		timestamp string
		want      bool
	}{
		{
			name: "a correctly signed delivery",
			body: body, signature: sign(secret, nowSec, body), timestamp: nowSec,
			want: true,
		},
		{
			name: "no signature header at all",
			body: body, timestamp: nowSec,
			want: false,
		},
		{
			name: "no timestamp header",
			body: body, signature: sign(secret, nowSec, body),
			want: false,
		},
		{
			name: "signed with a different secret",
			body: body, signature: sign("someone-else", nowSec, body), timestamp: nowSec,
			want: false,
		},
		{
			// The empty-secret guard is not an optimization: without it an
			// unconfigured deployment accepts anything signed with "".
			name:   "no secret is configured",
			secret: "-",
			body:   body, signature: sign("", nowSec, body), timestamp: nowSec,
			want: false,
		},
		{
			name: "a body that differs from the signed one by a single byte",
			body: `{"entry":[ ]}`, signature: sign(secret, nowSec, body), timestamp: nowSec,
			want: false,
		},
		{
			// The signature covers the timestamp, so moving the clock does not
			// only trip the skew check; it invalidates the digest too.
			name: "six minutes old",
			body: body, signature: sign(secret, sec(now.Add(-6*time.Minute)), body), timestamp: sec(now.Add(-6 * time.Minute)),
			want: false,
		},
		{
			name: "six minutes in the future",
			body: body, signature: sign(secret, sec(now.Add(6*time.Minute)), body), timestamp: sec(now.Add(6 * time.Minute)),
			want: false,
		},
		{
			name: "four minutes old",
			body: body, signature: sign(secret, sec(now.Add(-4*time.Minute)), body), timestamp: sec(now.Add(-4 * time.Minute)),
			want: true,
		},
		{
			// The comparison is a strict >, so the boundary itself is inside
			// the window. Both edges are asserted because an off-by-one here
			// is invisible until a delivery lands exactly on it.
			name: "exactly five minutes old",
			body: body, signature: sign(secret, sec(now.Add(-5*time.Minute)), body), timestamp: sec(now.Add(-5 * time.Minute)),
			want: true,
		},
		{
			name: "exactly five minutes in the future",
			body: body, signature: sign(secret, sec(now.Add(5*time.Minute)), body), timestamp: sec(now.Add(5 * time.Minute)),
			want: true,
		},
		{
			name: "five minutes and one second old",
			body: body, signature: sign(secret, sec(now.Add(-301*time.Second)), body), timestamp: sec(now.Add(-301 * time.Second)),
			want: false,
		},
		{
			name: "a signature without the sha256= prefix",
			body: body, signature: strings.TrimPrefix(sign(secret, nowSec, body), "sha256="), timestamp: nowSec,
			want: false,
		},
		{
			name: "a signature labelled with another algorithm",
			body: body, signature: "sha1=" + strings.TrimPrefix(sign(secret, nowSec, body), "sha256="), timestamp: nowSec,
			want: false,
		},
		{
			// Node's Buffer.from(hex) stops at the first bad character and
			// never throws; Go's hex.DecodeString returns an error. Both must
			// end in a plain false rather than a panic or a 500.
			name: "a signature that is not hex",
			body: body, signature: "sha256=zzzznothex", timestamp: nowSec,
			want: false,
		},
		{
			name: "a signature with an odd number of hex digits",
			body: body, signature: "sha256=abc", timestamp: nowSec,
			want: false,
		},
		{
			name: "a signature of the right shape but the wrong length",
			body: body, signature: "sha256=" + strings.Repeat("ab", 16), timestamp: nowSec,
			want: false,
		},
		{
			// Hex decoding is case-insensitive, so an upper-case digest from a
			// different router implementation still verifies.
			name: "an upper-case hex signature",
			body: body, signature: "sha256=" + strings.ToUpper(strings.TrimPrefix(sign(secret, nowSec, body), "sha256=")), timestamp: nowSec,
			want: true,
		},
		{
			name: "a non-numeric timestamp",
			body: body, signature: sign(secret, "not-a-number", body), timestamp: "not-a-number",
			want: false,
		},
		{
			name: "a negative timestamp",
			body: body, signature: sign(secret, "-"+nowSec, body), timestamp: "-" + nowSec,
			want: false,
		},
		{
			name: "a fractional timestamp",
			body: body, signature: sign(secret, nowSec+".5", body), timestamp: nowSec + ".5",
			want: false,
		},
		{
			name: "a timestamp padded with whitespace",
			body: body, signature: sign(secret, " "+nowSec, body), timestamp: " " + nowSec,
			want: false,
		},
		{
			// Longer than an int64 can hold. JavaScript coerces it to a huge
			// float and fails the skew check; Go must not treat the parse
			// failure as anything but a rejection.
			name: "a timestamp too large to parse",
			body: body, signature: sign(secret, strings.Repeat("9", 30), body), timestamp: strings.Repeat("9", 30),
			want: false,
		},
		{
			name: "an empty body, correctly signed",
			body: "", signature: sign(secret, nowSec, ""), timestamp: nowSec,
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := secret
			if tc.secret == "-" {
				key = ""
			}

			got := routersig.Valid(key, []byte(tc.body), headers(tc.signature, tc.timestamp), now)
			if got != tc.want {
				t.Errorf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The signed bytes are the body exactly as it arrived. Re-encoding a payload —
// the obvious thing to do once it has been parsed — changes key order and
// whitespace and breaks every signature, so the contract is asserted against a
// body no JSON encoder would reproduce.
func TestValidSignsTheBytesAsReceived(t *testing.T) {
	raw := "{\n  \"entry\" : [ ],\n  \"object\":\"whatsapp_business_account\"\n}"
	nowSec := strconv.FormatInt(now.Unix(), 10)

	if !routersig.Valid(secret, []byte(raw), headers(sign(secret, nowSec, raw), nowSec), now) {
		t.Error("a delivery signed over its exact bytes was rejected")
	}
}

func sec(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

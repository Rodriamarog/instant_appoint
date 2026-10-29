// Package routersig checks that a webhook delivery really came from the
// Neurocrow webhook router.
//
// Meta never reaches this app directly. hooks.neurocrow.com answers Meta's GET
// verification, checks Meta's own signature, and re-signs each delivery with a
// shared secret before forwarding it here. That re-signature is the only thing
// standing between a public URL and an attacker who can make this app store
// messages and spend Gemini and WhatsApp quota, so every rule below is a
// rejection rule and the default answer is no.
//
// This package is a pure port of lib/router-signature.ts and imports nothing
// outside the standard library, so the rules can be read against the
// TypeScript line by line.
package routersig

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The headers the router sets on every delivery.
const (
	SignatureHeader = "x-neurocrow-signature-256"
	TimestampHeader = "x-neurocrow-timestamp"

	// DeliveryIDHeader identifies one delivery attempt. Nothing reads it; it
	// is named here because it is part of the contract and shows up in logs.
	DeliveryIDHeader = "x-neurocrow-delivery-id"
)

// signaturePrefix labels the digest's algorithm. Only this one is accepted: a
// bare hex string or any other label is a different scheme, not a variation.
const signaturePrefix = "sha256="

// MaxSkew is how far a delivery's timestamp may be from this server's clock,
// in either direction. It bounds how long a captured delivery stays replayable
// against a server that has forgotten it.
//
// The comparison is strict, so a delivery exactly MaxSkew old is still
// accepted.
const MaxSkew = 5 * time.Minute

// Valid reports whether headers carry a signature this app's secret produces
// over rawBody, timestamped within MaxSkew of now.
//
// rawBody must be the request body exactly as it arrived. The digest covers
// those bytes, so a payload that has been through a JSON decoder and encoder —
// different key order, different whitespace — produces a different digest and
// is rejected even though it means the same thing.
func Valid(secret string, rawBody []byte, headers http.Header, now time.Time) bool {
	// An unset secret must reject everything rather than accept everything.
	// Without this, a deployment that forgot the environment variable verifies
	// deliveries against the empty key, which anyone can compute.
	if secret == "" {
		return false
	}

	signature := headers.Get(SignatureHeader)
	timestamp := headers.Get(TimestampHeader)
	if !strings.HasPrefix(signature, signaturePrefix) || !isDigits(timestamp) {
		return false
	}

	// isDigits has already ruled out a sign, a fraction and whitespace, so the
	// only parse failure left is a value too large for an int64 — far outside
	// any window, and a rejection either way.
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if skew := now.Unix() - seconds; skew > int64(MaxSkew.Seconds()) || skew < -int64(MaxSkew.Seconds()) {
		return false
	}

	// Malformed hex is a rejection, not an error worth reporting: a caller who
	// cannot produce hex cannot produce a signature either.
	got, err := hex.DecodeString(strings.TrimPrefix(signature, signaturePrefix))
	if err != nil {
		return false
	}

	// The timestamp is signed along with the body, which is what stops a
	// captured delivery from being replayed with a fresh timestamp.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte{'.'})
	mac.Write(rawBody)

	// hmac.Equal, not bytes.Equal: the comparison runs in time independent of
	// how many leading bytes matched, so a caller cannot discover the expected
	// digest one byte at a time by timing the responses.
	return hmac.Equal(got, mac.Sum(nil))
}

// isDigits reports whether value is one or more ASCII digits and nothing else,
// matching the /^\d+$/ the TypeScript applies to the timestamp header.
func isDigits(value string) bool {
	if value == "" {
		return false
	}

	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}

	return true
}

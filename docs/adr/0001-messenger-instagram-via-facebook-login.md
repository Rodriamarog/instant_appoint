# Messenger and Instagram through Facebook Login, on the WhatsApp app

Date: 2026-11-05. Status: accepted.

## Context

Clinics want the assistant on Messenger and Instagram as well as WhatsApp.
Meta offers two ways to reach Instagram messages: the Messenger Platform with
Facebook Login (the Instagram account must be professional and linked to a
Facebook Page), and the newer Instagram API with Instagram Login (no Page
needed).

Deliveries reach this app only through the Neurocrow webhook router, which
checks `X-Hub-Signature-256` against one Meta app secret.

## Decision

Both channels are connected with one Facebook login, on the same Meta app as
WhatsApp. It is the classic login asking for the permissions by name (as
Neurocrow's old site did); a Login for Business configuration can replace it
through `VITE_META_MESSAGING_CONFIG_ID` without code changes. The person picks one Page; the Page answers Messenger
and its linked Instagram account answers Instagram, both with the Page's
access token. Tokens are read with a long-lived user token, so they do not
expire.

## Consequences

- Webhooks are signed with the app secret the router already holds. Instagram
  Login signs with the Instagram app's own secret, which the router would
  reject.
- One button connects both channels, and nothing needs refreshing.
- An Instagram account that is not professional, or not linked to a Page,
  cannot be connected. The Integraciones page explains how to fix that.
- The customer is a page-scoped id, not a phone number, so conversations and
  messages carry a `channel`, and the receipt's quick replies name their
  appointment in the payload: these channels do not say which message a
  quick reply answers.
- Reminders stay on WhatsApp. The assistant asks Messenger and Instagram
  patients for a number.

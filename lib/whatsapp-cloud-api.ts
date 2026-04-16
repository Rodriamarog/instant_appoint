export async function sendWhatsAppCloudMessage(
  phoneNumberId: string,
  accessToken: string,
  to: string,
  text: string
): Promise<string> {
  const res = await fetch(`https://graph.facebook.com/v21.0/${phoneNumberId}/messages`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${accessToken}`,
    },
    body: JSON.stringify({
      messaging_product: 'whatsapp',
      to,
      type: 'text',
      text: { body: text },
    }),
  })

  const data = await res.json()
  if (!res.ok || data.error) {
    throw new Error(`WhatsApp send failed: ${JSON.stringify(data.error ?? data)}`)
  }

  return data.messages?.[0]?.id ?? ''
}

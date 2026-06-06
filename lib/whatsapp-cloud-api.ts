export async function sendWhatsAppTemplate(
  phoneNumberId: string,
  accessToken: string,
  to: string,
  templateName: string,
  languageCode: string,
  bodyParameters?: string[]
): Promise<string> {
  const templatePayload: Record<string, unknown> = {
    name: templateName,
    language: { code: languageCode },
  }

  if (bodyParameters && bodyParameters.length > 0) {
    templatePayload.components = [
      {
        type: 'body',
        parameters: bodyParameters.map(text => ({ type: 'text', text })),
      },
    ]
  }

  const res = await fetch(`https://graph.facebook.com/v21.0/${phoneNumberId}/messages`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${accessToken}`,
    },
    body: JSON.stringify({
      messaging_product: 'whatsapp',
      to,
      type: 'template',
      template: templatePayload,
    }),
  })

  const data = await res.json()
  if (!res.ok || data.error) {
    throw new Error(`WhatsApp template send failed: ${JSON.stringify(data.error ?? data)}`)
  }

  return data.messages?.[0]?.id ?? ''
}

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

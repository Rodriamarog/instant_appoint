/**
 * The message-template endpoints, and the two bits of reasoning the page does
 * about a template before it calls them.
 *
 * Every call goes to the Go server rather than to Meta: the account's access
 * token stays there, and the server resolves which WhatsApp Business account
 * the caller owns from their session. Nothing here names an account.
 */
import { call } from '@/lib/api'

/** A template as Meta describes it, passed through by the server verbatim. */
export interface MessageTemplate {
  name: string
  status: string
  category: string
  language: string
  components?: { type: string; text?: string }[]
}

/** What the create form submits. */
export interface TemplateDraft {
  name: string
  category: string
  language: string
  body: string
}

export const CATEGORIES = ['UTILITY', 'MARKETING', 'AUTHENTICATION']

export const LANGUAGES = [
  { code: 'es', label: 'Spanish (es)' },
  { code: 'en_US', label: 'English (en_US)' },
  { code: 'pt_BR', label: 'Portuguese (pt_BR)' },
]

/** The body text, which is the only component this app creates or renders. */
export function templateBody(template: MessageTemplate): string {
  const body = template.components?.find(
    (component) => component.type?.toUpperCase() === 'BODY',
  )

  return body?.text ?? ''
}

/**
 * The placeholder numbers in a body, ascending and without repeats.
 *
 * Meta fills {{1}}, {{2}} positionally from the parameters a send carries, so
 * this is what says how many values a send has to supply. A body that repeats
 * {{1}} still takes one value.
 */
export function placeholders(body: string): number[] {
  const found = new Set<number>()

  for (const match of body.matchAll(/\{\{\s*(\d+)\s*\}\}/g)) {
    found.add(Number(match[1]))
  }

  return [...found].sort((a, b) => a - b)
}

/**
 * What is wrong with a draft, by field, or nothing when it is submittable.
 *
 * Meta answers a bad name with a round trip and an opaque rejection, so the
 * form checks first; the server checks again, because it is reachable without
 * the form.
 */
export function validateDraft(
  draft: TemplateDraft,
): Partial<Record<keyof TemplateDraft, string>> {
  const problems: Partial<Record<keyof TemplateDraft, string>> = {}

  if (!draft.name.trim()) {
    problems.name = 'A name is required.'
  } else if (!/^[a-z0-9_]+$/.test(draft.name)) {
    problems.name = 'Use lowercase letters, numbers and underscores only.'
  }

  if (!CATEGORIES.includes(draft.category)) {
    problems.category = 'Choose a category.'
  }

  if (!LANGUAGES.some((language) => language.code === draft.language)) {
    problems.language = 'Choose a language.'
  }

  if (!draft.body.trim()) {
    problems.body = 'A body is required.'
  }

  return problems
}

export async function listTemplates(): Promise<MessageTemplate[]> {
  const body = await call<{ templates?: MessageTemplate[] }>(
    '/api/whatsapp/templates',
  )

  return body.templates ?? []
}

export async function createTemplate(draft: TemplateDraft): Promise<void> {
  await call('/api/whatsapp/templates/create', {
    method: 'POST',
    body: JSON.stringify(draft),
  })
}

/**
 * Sends one template and returns the WhatsApp message id.
 *
 * bodyParameters fill the placeholders in order. A template without any sends
 * an empty array, which the server turns into a request with no components —
 * Meta rejects an empty component list.
 */
export async function sendTemplate(send: {
  name: string
  language: string
  to: string
  bodyParameters: string[]
}): Promise<string> {
  const body = await call<{ message_id?: string }>(
    '/api/whatsapp/templates/send',
    {
      method: 'POST',
      body: JSON.stringify({
        template_name: send.name,
        language_code: send.language,
        to: send.to,
        body_parameters: send.bodyParameters,
      }),
    },
  )

  return body.message_id ?? ''
}

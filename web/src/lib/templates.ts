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
  components?: {
    type: string
    text?: string
    buttons?: { type: string; text?: string }[]
  }[]
}

/** What the create form submits. */
export interface TemplateDraft {
  name: string
  category: string
  language: string
  body: string

  /** Quick-reply labels, in the order they sit under the message. */
  buttons: string[]
}

export const CATEGORIES = ['UTILITY', 'MARKETING', 'AUTHENTICATION']

export const LANGUAGES = [
  { code: 'es', label: 'Español (es)' },
  { code: 'en_US', label: 'Inglés (en_US)' },
  { code: 'pt_BR', label: 'Portugués (pt_BR)' },
]

/** Meta's own limits on a row of quick replies. */
export const MAX_BUTTONS = 3
export const MAX_BUTTON_LABEL = 25

/**
 * How long a button's label is, counted the way the Go handler counts it.
 *
 * String.length counts UTF-16 units, so an emoji would read as two, and the
 * handler's own check (meta.ReplyButtonLabelFits) counts characters. Letting
 * the two disagree is how a form accepts wording the server then refuses,
 * which is exactly what happened while this counted UTF-16 units and the
 * handler counted bytes. Spreading the string counts code points, which is
 * the same number both sides now hold the label to.
 */
export function buttonLabelLength(label: string): number {
  return [...label].length
}

/**
 * The Confirm and Cancel wording suggested for each language on offer.
 *
 * The buttons live inside the template, so Meta approves them once, in one
 * language, and a send cannot swap them for the recipient's. Offering the
 * matching pair with the language is the only point at which that is cheap
 * to get right — afterwards it costs a new template and another review.
 */
export const QUICK_REPLIES: Record<string, [string, string]> = {
  es: ['Confirmar', 'Cancelar'],
  en_US: ['Confirm', 'Cancel'],
  pt_BR: ['Confirmar', 'Cancelar'],
}

/** The body text, the component that carries the words of the message. */
export function templateBody(template: MessageTemplate): string {
  const body = template.components?.find(
    (component) => component.type?.toUpperCase() === 'BODY',
  )

  return body?.text ?? ''
}

/**
 * The template's quick-reply labels, in order.
 *
 * Order is the whole identity of a quick reply: a tap is reported by the
 * button's index under the message, so a list read out of order would name
 * the wrong one. A template with no buttons gives an empty list rather than
 * nothing, because most of them have none.
 */
export function templateButtons(template: MessageTemplate): string[] {
  const buttons = template.components?.find(
    (component) => component.type?.toUpperCase() === 'BUTTONS',
  )

  return (buttons?.buttons ?? []).map((button) => button.text ?? '')
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
    problems.name = 'El nombre es obligatorio.'
  } else if (!/^[a-z0-9_]+$/.test(draft.name)) {
    problems.name = 'Usa solo minúsculas, números y guiones bajos.'
  }

  if (!CATEGORIES.includes(draft.category)) {
    problems.category = 'Elige una categoría.'
  }

  if (!LANGUAGES.some((language) => language.code === draft.language)) {
    problems.language = 'Elige un idioma.'
  }

  if (!draft.body.trim()) {
    problems.body = 'El mensaje es obligatorio.'
  }

  // Meta answers a bad button with the same opaque rejection as a bad name,
  // except the cost is higher: by then the person has usually decided the
  // wording and moved on.
  if (draft.buttons.length > MAX_BUTTONS) {
    problems.buttons = 'Un mensaje admite como máximo tres botones.'
  } else if (draft.buttons.some((label) => !label.trim())) {
    problems.buttons = 'Cada botón necesita un texto.'
  } else if (
    draft.buttons.some((label) => buttonLabelLength(label) > MAX_BUTTON_LABEL)
  ) {
    problems.buttons = 'El texto de un botón no puede pasar de 25 caracteres.'
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

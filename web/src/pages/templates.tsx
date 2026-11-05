import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router'

import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { errorMessage } from '@/lib/errors'
import {
  CATEGORIES,
  LANGUAGES,
  MAX_BUTTON_LABEL,
  type MessageTemplate,
  QUICK_REPLIES,
  type TemplateDraft,
  createTemplate,
  listTemplates,
  placeholders,
  sendTemplate,
  templateBody,
  templateButtons,
  validateDraft,
} from '@/lib/templates'
import { listConnectedAccounts } from '@/lib/whatsapp-accounts'

const emptyDraft: TemplateDraft = {
  name: '',
  category: 'UTILITY',
  language: 'es',
  body: '',
  buttons: [],
}

/** The Confirm and Cancel pair to offer for a language the form can choose. */
function suggestedButtons(language: string): string[] {
  return [...(QUICK_REPLIES[language] ?? QUICK_REPLIES.es)]
}

/** Meta's approval states, as a colour. Anything unexpected reads as a problem. */
function statusClass(status: string) {
  if (status === 'APPROVED') {
    return 'bg-green-100 text-green-700 dark:bg-green-500/15 dark:text-green-300'
  }
  if (status === 'PENDING' || status === 'IN_APPEAL') {
    return 'bg-muted text-muted-foreground'
  }

  return 'bg-red-100 text-red-600 dark:bg-red-500/15 dark:text-red-300'
}

export function TemplatesPage() {
  const [hasAccount, setHasAccount] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const [templates, setTemplates] = useState<MessageTemplate[]>([])
  const [listError, setListError] = useState('')

  const load = useCallback(async () => {
    try {
      // Whether there is an account at all is the collection's answer; the
      // template endpoints resolve which one again from the session. Asking
      // here only decides whether to show the page or point at the setup one.
      const accounts = await listConnectedAccounts()

      setHasAccount(accounts.length > 0)
      if (accounts.length === 0) {
        return
      }

      setTemplates(await listTemplates())
      setListError('')
    } catch (error) {
      setListError(
        errorMessage(error, 'No se pudieron cargar tus plantillas.'),
      )
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Cargando plantillas…</p>
  }

  if (!hasAccount) {
    return (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Conecta un número de WhatsApp Business antes de crear plantillas.
        </p>
        <Link to="/configuracion/whatsapp" className="text-sm underline">
          Ir a la conexión de WhatsApp
        </Link>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm text-muted-foreground">
          Las plantillas de mensaje viven en Meta. Una nueva se queda en
          PENDING hasta que Meta la aprueba, y solo se puede enviar una
          plantilla aprobada.
        </p>
      </div>

      <CreateTemplateForm onCreated={load} />

      {listError && <Alert variant="destructive">{listError}</Alert>}

      <Card>
        <CardHeader>
          <CardTitle>Tus plantillas</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {templates.length === 0 && !listError && (
            <p className="text-sm text-muted-foreground">
              Aún no hay plantillas. Crea una aquí arriba.
            </p>
          )}
          {templates.map((template) => (
            <TemplateRow
              key={`${template.name}:${template.language}`}
              template={template}
            />
          ))}
        </CardContent>
      </Card>
    </div>
  )
}

/** The create form. Nothing is sent until every field is one Meta accepts. */
function CreateTemplateForm({ onCreated }: { onCreated: () => Promise<void> }) {
  const [isOpen, setIsOpen] = useState(false)
  const [draft, setDraft] = useState(emptyDraft)
  const [problems, setProblems] = useState<
    Partial<Record<keyof TemplateDraft, string>>
  >({})
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [failure, setFailure] = useState('')
  const [success, setSuccess] = useState('')

  // Changing the language carries the suggested buttons with it, because a
  // Spanish "Confirmar" sitting under an English template is approved that
  // way and cannot be reworded per recipient afterwards. Wording the person
  // typed themselves is theirs, so only labels still equal to the previous
  // language's suggestion are replaced.
  const chooseLanguage = (language: string) => {
    const previous = suggestedButtons(draft.language)
    const untouched = draft.buttons.every(
      (label, index) => label === previous[index],
    )

    setDraft({
      ...draft,
      language,
      buttons: untouched
        ? suggestedButtons(language).slice(0, draft.buttons.length)
        : draft.buttons,
    })
  }

  const submit = async () => {
    const found = validateDraft(draft)
    setProblems(found)
    setFailure('')
    setSuccess('')
    if (Object.keys(found).length > 0) {
      return
    }

    setIsSubmitting(true)
    try {
      await createTemplate(draft)
      setSuccess(
        `"${draft.name}" se envió a aprobación. Se queda en PENDING hasta que Meta la apruebe.`,
      )
      setDraft(emptyDraft)
      setIsOpen(false)
      await onCreated()
    } catch (error) {
      setFailure(errorMessage(error, 'No se pudo enviar la plantilla.'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="space-y-3">
      <Button
        type="button"
        onClick={() => {
          setIsOpen((open) => !open)
          setFailure('')
          setSuccess('')
        }}
      >
        {isOpen ? 'Cancelar' : 'Nueva plantilla'}
      </Button>

      {success && <Alert variant="success" role="status">{success}</Alert>}
      {failure && <Alert variant="destructive">{failure}</Alert>}

      {isOpen && (
        <Card>
          <CardHeader>
            <CardTitle>Nueva plantilla</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-3">
              <div className="space-y-1">
                <Label htmlFor="template-name">Nombre</Label>
                <Input
                  id="template-name"
                  value={draft.name}
                  placeholder="recordatorio_de_cita"
                  onChange={(event) =>
                    setDraft({ ...draft, name: event.target.value })
                  }
                />
                <FieldProblem problem={problems.name} />
              </div>

              <div className="space-y-1">
                <Label htmlFor="template-category">Categoría</Label>
                <select
                  id="template-category"
                  className="h-9 w-full rounded-md border px-2 text-sm"
                  value={draft.category}
                  onChange={(event) =>
                    setDraft({ ...draft, category: event.target.value })
                  }
                >
                  {CATEGORIES.map((category) => (
                    <option key={category} value={category}>
                      {category}
                    </option>
                  ))}
                </select>
                <FieldProblem problem={problems.category} />
              </div>

              <div className="space-y-1">
                <Label htmlFor="template-language">Idioma</Label>
                <select
                  id="template-language"
                  className="h-9 w-full rounded-md border px-2 text-sm"
                  value={draft.language}
                  onChange={(event) => chooseLanguage(event.target.value)}
                >
                  {LANGUAGES.map((language) => (
                    <option key={language.code} value={language.code}>
                      {language.label}
                    </option>
                  ))}
                </select>
                <FieldProblem problem={problems.language} />
              </div>
            </div>

            <div className="space-y-1">
              <Label htmlFor="template-body">Mensaje</Label>
              <Textarea
                id="template-body"
                rows={3}
                value={draft.body}
                placeholder="Hola {{1}}, te recordamos tu cita {{2}}."
                onChange={(event) =>
                  setDraft({ ...draft, body: event.target.value })
                }
              />
              <p className="text-xs text-muted-foreground">
                Usa {'{{1}}'}, {'{{2}}'} donde vaya un valor.
              </p>
              <FieldProblem problem={problems.body} />
            </div>

            <div className="space-y-2">
              <label className="flex items-center gap-2 text-sm font-medium">
                <input
                  type="checkbox"
                  className="size-4"
                  checked={draft.buttons.length > 0}
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      buttons: event.target.checked
                        ? suggestedButtons(draft.language)
                        : [],
                    })
                  }
                />
                Botones de respuesta rápida
              </label>
              <p className="text-xs text-muted-foreground">
                El paciente responde con un toque en vez de escribir. Los
                botones se aprueban junto con la plantilla: no se pueden
                añadir ni cambiar después.
              </p>

              {draft.buttons.length > 0 && (
                <div className="grid gap-3 sm:grid-cols-2">
                  {draft.buttons.map((label, index) => (
                    <div key={index} className="space-y-1">
                      <Label htmlFor={`template-button-${index + 1}`}>
                        {`Botón ${index + 1}`}
                      </Label>
                      <Input
                        id={`template-button-${index + 1}`}
                        value={label}
                        maxLength={MAX_BUTTON_LABEL}
                        onChange={(event) =>
                          setDraft({
                            ...draft,
                            buttons: draft.buttons.map((existing, at) =>
                              at === index ? event.target.value : existing,
                            ),
                          })
                        }
                      />
                    </div>
                  ))}
                </div>
              )}

              <FieldProblem problem={problems.buttons} />
            </div>

            <Button type="button" onClick={submit} disabled={isSubmitting}>
              {isSubmitting ? 'Enviando…' : 'Enviar a aprobación'}
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  )
}

function FieldProblem({ problem }: { problem?: string }) {
  if (!problem) {
    return null
  }

  return <p className="text-xs text-destructive">{problem}</p>
}

/** One template, with the test send Meta allows only for an approved one. */
function TemplateRow({ template }: { template: MessageTemplate }) {
  const body = templateBody(template)
  const slots = placeholders(body)
  const buttons = templateButtons(template)

  const [isSending, setIsSending] = useState(false)
  const [isOpen, setIsOpen] = useState(false)
  const [to, setTo] = useState('')
  const [values, setValues] = useState<Record<number, string>>({})
  const [feedback, setFeedback] = useState<{ ok: boolean; text: string } | null>(
    null,
  )

  const send = async () => {
    if (!to.trim()) {
      setFeedback({ ok: false, text: 'Escribe el número al que enviar.' })
      return
    }
    // Meta rejects the whole send if a placeholder has no value, so the
    // missing one is worth saying before the round trip.
    if (slots.some((slot) => !(values[slot] ?? '').trim())) {
      setFeedback({
        ok: false,
        text: 'Rellena todos los valores antes de enviar.',
      })
      return
    }

    setIsSending(true)
    setFeedback(null)
    try {
      const messageID = await sendTemplate({
        name: template.name,
        language: template.language,
        to: to.trim(),
        bodyParameters: slots.map((slot) => values[slot].trim()),
      })
      setFeedback({ ok: true, text: `Mensaje enviado (${messageID}).` })
      setIsOpen(false)
      setTo('')
      setValues({})
    } catch (error) {
      setFeedback({
        ok: false,
        text: errorMessage(error, 'No se pudo enviar.'),
      })
    } finally {
      setIsSending(false)
    }
  }

  return (
    <div className="space-y-2 rounded-lg border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">{template.name}</span>
          <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
            {template.language}
          </span>
          <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
            {template.category}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span
            className={`rounded px-1.5 py-0.5 text-xs ${statusClass(template.status)}`}
          >
            {template.status}
          </span>
          {template.status === 'APPROVED' && (
            <Button
              type="button"
              size="xs"
              onClick={() => {
                setIsOpen((open) => !open)
                setFeedback(null)
              }}
            >
              {isOpen ? 'Cerrar' : 'Enviar prueba'}
            </Button>
          )}
        </div>
      </div>

      {body && (
        <p className="rounded bg-muted p-2 text-xs text-muted-foreground">{body}</p>
      )}

      {/*
        Shown as the patient will see them. Approval is what freezes the
        buttons, so between submitting and the status turning APPROVED this
        is the last chance to notice they are wrong — after that, fixing them
        means a differently named template and another review.
      */}
      {buttons.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {buttons.map((label, index) => (
            <span
              key={`${index}:${label}`}
              className="rounded border px-2 py-0.5 text-xs text-muted-foreground"
            >
              {label}
            </span>
          ))}
        </div>
      )}

      {isOpen && (
        <div className="space-y-2 pt-1">
          <div className="space-y-1">
            <Label htmlFor={`send-to-${template.name}`}>Enviar a</Label>
            <Input
              id={`send-to-${template.name}`}
              value={to}
              placeholder="Código de país y número, solo dígitos"
              onChange={(event) => setTo(event.target.value)}
            />
          </div>

          {slots.map((slot) => (
            <div key={slot} className="space-y-1">
              <Label htmlFor={`param-${template.name}-${slot}`}>
                {`{{${slot}}}`}
              </Label>
              <Input
                id={`param-${template.name}-${slot}`}
                value={values[slot] ?? ''}
                onChange={(event) =>
                  setValues({ ...values, [slot]: event.target.value })
                }
              />
            </div>
          ))}

          <Button type="button" onClick={send} disabled={isSending}>
            {isSending ? 'Enviando…' : 'Enviar'}
          </Button>
        </div>
      )}

      {feedback && (
        <p
          className={`text-sm ${feedback.ok ? 'text-green-600 dark:text-green-400' : 'text-destructive'}`}
        >
          {feedback.text}
        </p>
      )}
    </div>
  )
}

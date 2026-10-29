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
  type MessageTemplate,
  type TemplateDraft,
  createTemplate,
  listTemplates,
  placeholders,
  sendTemplate,
  templateBody,
  validateDraft,
} from '@/lib/templates'
import { listConnectedAccounts } from '@/lib/whatsapp-accounts'

const emptyDraft: TemplateDraft = {
  name: '',
  category: 'UTILITY',
  language: 'es',
  body: '',
}

/** Meta's approval states, as a colour. Anything unexpected reads as a problem. */
function statusClass(status: string) {
  if (status === 'APPROVED') {
    return 'bg-green-100 text-green-700'
  }
  if (status === 'PENDING' || status === 'IN_APPEAL') {
    return 'bg-gray-100 text-gray-600'
  }

  return 'bg-red-100 text-red-600'
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
        errorMessage(error, 'Could not load your templates.'),
      )
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading templates…</p>
  }

  if (!hasAccount) {
    return (
      <div className="space-y-3">
        <h2 className="font-heading text-2xl font-bold">Templates</h2>
        <p className="text-sm text-muted-foreground">
          Connect a WhatsApp Business number before creating templates.
        </p>
        <Link to="/whatsapp" className="text-sm underline">
          Go to WhatsApp setup
        </Link>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="font-heading text-2xl font-bold">Templates</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Message templates live at Meta. A new one stays PENDING until Meta
          approves it, and only an approved template can be sent.
        </p>
      </div>

      <CreateTemplateForm onCreated={load} />

      {listError && <Alert variant="destructive">{listError}</Alert>}

      <Card>
        <CardHeader>
          <CardTitle>Your templates</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {templates.length === 0 && !listError && (
            <p className="text-sm text-muted-foreground">
              No templates yet. Create one above.
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
        `"${draft.name}" was submitted for approval. It stays PENDING until Meta approves it.`,
      )
      setDraft(emptyDraft)
      setIsOpen(false)
      await onCreated()
    } catch (error) {
      setFailure(errorMessage(error, 'Submitting the template failed.'))
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
        {isOpen ? 'Cancel' : 'New template'}
      </Button>

      {success && <Alert>{success}</Alert>}
      {failure && <Alert variant="destructive">{failure}</Alert>}

      {isOpen && (
        <Card>
          <CardHeader>
            <CardTitle>New template</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-3">
              <div className="space-y-1">
                <Label htmlFor="template-name">Name</Label>
                <Input
                  id="template-name"
                  value={draft.name}
                  placeholder="appointment_reminder"
                  onChange={(event) =>
                    setDraft({ ...draft, name: event.target.value })
                  }
                />
                <FieldProblem problem={problems.name} />
              </div>

              <div className="space-y-1">
                <Label htmlFor="template-category">Category</Label>
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
                <Label htmlFor="template-language">Language</Label>
                <select
                  id="template-language"
                  className="h-9 w-full rounded-md border px-2 text-sm"
                  value={draft.language}
                  onChange={(event) =>
                    setDraft({ ...draft, language: event.target.value })
                  }
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
              <Label htmlFor="template-body">Body</Label>
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
                Use {'{{1}}'}, {'{{2}}'} where a value goes.
              </p>
              <FieldProblem problem={problems.body} />
            </div>

            <Button type="button" onClick={submit} disabled={isSubmitting}>
              {isSubmitting ? 'Submitting…' : 'Submit for approval'}
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

  const [isSending, setIsSending] = useState(false)
  const [isOpen, setIsOpen] = useState(false)
  const [to, setTo] = useState('')
  const [values, setValues] = useState<Record<number, string>>({})
  const [feedback, setFeedback] = useState<{ ok: boolean; text: string } | null>(
    null,
  )

  const send = async () => {
    if (!to.trim()) {
      setFeedback({ ok: false, text: 'Enter a number to send to.' })
      return
    }
    // Meta rejects the whole send if a placeholder has no value, so the
    // missing one is worth saying before the round trip.
    if (slots.some((slot) => !(values[slot] ?? '').trim())) {
      setFeedback({ ok: false, text: 'Fill every placeholder before sending.' })
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
      setFeedback({ ok: true, text: `Message sent (${messageID}).` })
      setIsOpen(false)
      setTo('')
      setValues({})
    } catch (error) {
      setFeedback({
        ok: false,
        text: errorMessage(error, 'Sending failed.'),
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
          <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500">
            {template.language}
          </span>
          <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500">
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
              {isOpen ? 'Close' : 'Send test'}
            </Button>
          )}
        </div>
      </div>

      {body && (
        <p className="rounded bg-gray-50 p-2 text-xs text-gray-600">{body}</p>
      )}

      {isOpen && (
        <div className="space-y-2 pt-1">
          <div className="space-y-1">
            <Label htmlFor={`send-to-${template.name}`}>Send to</Label>
            <Input
              id={`send-to-${template.name}`}
              value={to}
              placeholder="Country code and number, digits only"
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
            {isSending ? 'Sending…' : 'Send'}
          </Button>
        </div>
      )}

      {feedback && (
        <p
          className={`text-sm ${feedback.ok ? 'text-green-600' : 'text-destructive'}`}
        >
          {feedback.text}
        </p>
      )}
    </div>
  )
}

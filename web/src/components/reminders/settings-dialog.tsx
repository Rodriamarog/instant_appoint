import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { errorMessage } from '@/lib/errors'
import {
  TIMEZONE_OPTIONS,
  TIMING_OPTIONS,
  type ReminderSettings,
} from '@/lib/reminder-settings'
import {
  listTemplates,
  placeholders,
  templateBody,
  type MessageTemplate,
} from '@/lib/templates'

/** How many values a reminder fills in: the client's name, the date, the time. */
const REMINDER_PARAMETERS = 3

/**
 * A template and its language as one value a select can carry.
 *
 * They travel together because a template name alone does not identify a
 * template: the same name exists once per language, and Meta refuses a send
 * that names the wrong pair.
 */
function templateValue(name: string, language: string) {
  return `${name}\u0000${language}`
}

/**
 * What a person decides about their reminders: how far ahead they go out,
 * whether they go out at all, which approved template carries them, and the
 * zone their date and time are spelled in.
 *
 * The template cannot be typed. Outside the 24-hour window a customer's own
 * message opens, WhatsApp delivers nothing but a template Meta has approved,
 * so a free-text name would only produce sends Meta rejects — the list is the
 * account's own approved templates.
 */
export function ReminderSettingsDialog({
  open,
  onOpenChange,
  settings,
  onSave,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  settings: ReminderSettings
  onSave: (settings: ReminderSettings) => Promise<void>
}) {
  // Edited locally so Cancel really cancels, and so the upcoming list does
  // not lurch about while somebody is still deciding.
  const [draft, setDraft] = useState(settings)
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState('')

  // null while the answer is unknown, so an account with no templates is not
  // told so during the round trip that would have found them.
  const [templates, setTemplates] = useState<MessageTemplate[] | null>(null)
  const [templatesError, setTemplatesError] = useState('')

  useEffect(() => {
    if (open) {
      setDraft(settings)
      setError('')
    }
  }, [open, settings])

  useEffect(() => {
    if (!open) {
      return
    }

    let current = true
    setTemplatesError('')

    listTemplates()
      .then((all) => {
        if (current) {
          // Only an approved template can be delivered; a pending or a
          // rejected one would be refused at send time.
          setTemplates(all.filter((template) => template.status === 'APPROVED'))
        }
      })
      .catch(() => {
        if (current) {
          setTemplates([])
          setTemplatesError(
            'Could not load your approved templates. Connect a WhatsApp number, or try again.',
          )
        }
      })

    return () => {
      current = false
    }
  }, [open])

  const chosen = templates?.find(
    (template) =>
      template.name === draft.template_name &&
      template.language === draft.template_language,
  )
  const body = chosen ? templateBody(chosen) : ''
  const values = chosen ? placeholders(body).length : REMINDER_PARAMETERS

  // A zone already saved that is not on the short list is kept rather than
  // silently swapped for the first one that is.
  const zones = TIMEZONE_OPTIONS.includes(draft.timezone)
    ? TIMEZONE_OPTIONS
    : [draft.timezone, ...TIMEZONE_OPTIONS]

  const save = async () => {
    setIsSaving(true)
    try {
      await onSave(draft)
      onOpenChange(false)
    } catch (err) {
      setError(errorMessage(err, 'Could not save your settings.'))
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Reminder settings</DialogTitle>
          <DialogDescription>
            When a reminder goes out, whether it goes out at all, and what carries it.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5 pt-2">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Enable reminders</span>
            <button
              type="button"
              role="switch"
              aria-checked={draft.is_active}
              aria-label={draft.is_active ? 'Disable reminders' : 'Enable reminders'}
              onClick={() => setDraft((d) => ({ ...d, is_active: !d.is_active }))}
              className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus-visible:ring-2 focus-visible:ring-ring ${
                draft.is_active ? 'bg-primary' : 'bg-muted'
              }`}
            >
              <span
                className={`inline-block h-4 w-4 transform rounded-full bg-background shadow transition-transform ${
                  draft.is_active ? 'translate-x-6' : 'translate-x-1'
                }`}
              />
            </button>
          </div>

          <fieldset className="space-y-3">
            <legend className="text-sm font-medium">
              Send the reminder this far ahead
            </legend>
            <div className="flex flex-wrap gap-2">
              {TIMING_OPTIONS.map((option) => (
                <button
                  key={option.minutes}
                  type="button"
                  aria-pressed={draft.timing_minutes === option.minutes}
                  onClick={() =>
                    setDraft((d) => ({ ...d, timing_minutes: option.minutes }))
                  }
                  className={`rounded-lg border px-4 py-2 text-sm font-medium transition-colors ${
                    draft.timing_minutes === option.minutes
                      ? 'border-primary bg-primary text-primary-foreground'
                      : 'border-border hover:border-foreground/40'
                  }`}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </fieldset>

          <div className="space-y-2">
            <label htmlFor="reminder-template" className="text-sm font-medium">
              Approved template
            </label>

            {templates === null ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading your approved templates...
              </div>
            ) : (
              <select
                id="reminder-template"
                value={templateValue(draft.template_name, draft.template_language)}
                onChange={(event) => {
                  const [name, language] = event.target.value.split('\u0000')
                  setDraft((d) => ({
                    ...d,
                    template_name: name,
                    template_language: language || d.template_language,
                  }))
                }}
                className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
              >
                <option value={templateValue('', draft.template_language)}>
                  No template chosen
                </option>
                {draft.template_name !== '' && !chosen && (
                  <option
                    value={templateValue(draft.template_name, draft.template_language)}
                  >
                    {draft.template_name} ({draft.template_language})
                  </option>
                )}
                {templates.map((template) => (
                  <option
                    key={templateValue(template.name, template.language)}
                    value={templateValue(template.name, template.language)}
                  >
                    {template.name} ({template.language})
                  </option>
                ))}
              </select>
            )}

            {templatesError && <p className="text-sm text-destructive">{templatesError}</p>}

            {templates !== null && templates.length === 0 && !templatesError && (
              <p className="text-xs text-muted-foreground">
                You have no approved templates yet. Create one on the Templates page and
                wait for Meta to approve it.
              </p>
            )}

            {body && <p className="rounded-lg bg-muted p-3 text-xs">{body}</p>}

            {chosen && values !== REMINDER_PARAMETERS && (
              <p className="text-xs text-destructive">
                A reminder fills in three values — the name, the date and the time — but
                this template takes {values}. Meta will refuse the send.
              </p>
            )}

            {draft.template_name === '' && (
              <p className="text-xs text-muted-foreground">
                Reminders will not send until you choose a template. WhatsApp only
                delivers approved templates outside a conversation.
              </p>
            )}
          </div>

          <div className="space-y-2">
            <label htmlFor="reminder-timezone" className="text-sm font-medium">
              Spell the date and time in
            </label>
            <select
              id="reminder-timezone"
              value={draft.timezone}
              onChange={(event) =>
                setDraft((d) => ({ ...d, timezone: event.target.value }))
              }
              className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
            >
              {zones.map((zone) => (
                <option key={zone} value={zone}>
                  {zone.replace('_', ' ')}
                </option>
              ))}
            </select>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="flex justify-end gap-2 pt-1">
            <Button
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isSaving}
            >
              Cancel
            </Button>
            <Button onClick={save} disabled={isSaving}>
              {isSaving ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                'Save'
              )}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

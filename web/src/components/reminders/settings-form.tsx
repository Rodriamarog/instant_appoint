import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'
import { errorMessage } from '@/lib/errors'
import { TIMING_OPTIONS, type BusinessSettings } from '@/lib/business-settings'
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
 * What a person decides about their reminders: whether they go out at all,
 * how far ahead, and which approved template carries them. It sits inline in
 * Configuración; the zone the date and time are spelled in is the business's
 * own, under Configuración › Negocio.
 *
 * The template cannot be typed. Outside the 24-hour window a customer's own
 * message opens, WhatsApp delivers nothing but a template Meta has approved,
 * so a free-text name would only produce sends Meta rejects — the list is the
 * account's own approved templates.
 */
export function ReminderSettingsForm({
  settings,
  onSave,
}: {
  settings: BusinessSettings
  onSave: (settings: BusinessSettings) => Promise<void>
}) {
  // Edited locally so nothing is written until Guardar.
  const [draft, setDraft] = useState(settings)
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  // null while the answer is unknown, so an account with no templates is not
  // told so during the round trip that would have found them.
  const [templates, setTemplates] = useState<MessageTemplate[] | null>(null)
  const [templatesError, setTemplatesError] = useState('')

  useEffect(() => {
    setDraft(settings)
  }, [settings])

  useEffect(() => {
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
            'No se pudieron cargar tus plantillas aprobadas. Conecta un número de WhatsApp o inténtalo de nuevo.',
          )
        }
      })

    return () => {
      current = false
    }
  }, [])

  const chosen = templates?.find(
    (template) =>
      template.name === draft.template_name &&
      template.language === draft.template_language,
  )
  const body = chosen ? templateBody(chosen) : ''
  const values = chosen ? placeholders(body).length : REMINDER_PARAMETERS

  const dirty =
    draft.is_active !== settings.is_active ||
    draft.timing_minutes !== settings.timing_minutes ||
    draft.template_name !== settings.template_name ||
    draft.template_language !== settings.template_language

  const change = (patch: Partial<BusinessSettings>) => {
    setDraft((d) => ({ ...d, ...patch }))
    setSaved(false)
  }

  const save = async () => {
    setIsSaving(true)
    try {
      await onSave(draft)
      setSaved(true)
    } catch (err) {
      setError(errorMessage(err, 'No se pudieron guardar tus ajustes.'))
    } finally {
      setIsSaving(false)
    }
  }

  return (
        <div className="space-y-5">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Enviar recordatorios</span>
            <button
              type="button"
              role="switch"
              aria-checked={draft.is_active}
              aria-label={
                draft.is_active
                  ? 'Apagar los recordatorios'
                  : 'Encender los recordatorios'
              }
              onClick={() => change({ is_active: !draft.is_active })}
              className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand ${
                draft.is_active ? 'bg-brand' : 'bg-border'
              }`}
            >
              <span
                className={`inline-block size-5 transform rounded-full bg-white shadow-sm transition-transform ${
                  draft.is_active ? 'translate-x-[22px]' : 'translate-x-0.5'
                }`}
              />
            </button>
          </div>

          <fieldset className="space-y-3">
            <legend className="text-sm font-medium">
              Enviar el recordatorio con esta antelación
            </legend>
            <div className="flex flex-wrap gap-2">
              {TIMING_OPTIONS.map((option) => (
                <button
                  key={option.minutes}
                  type="button"
                  aria-pressed={draft.timing_minutes === option.minutes}
                  onClick={() => change({ timing_minutes: option.minutes })}
                  className={`cursor-pointer rounded-lg border px-4 py-2 text-sm font-medium transition-colors ${
                    draft.timing_minutes === option.minutes
                      ? 'border-brand bg-accent text-accent-foreground ring-1 ring-brand'
                      : 'border-border hover:border-brand/40'
                  }`}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </fieldset>

          <div className="space-y-2">
            <label htmlFor="reminder-template" className="text-sm font-medium">
              Plantilla aprobada
            </label>

            {templates === null ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Cargando tus plantillas aprobadas...
              </div>
            ) : (
              <select
                id="reminder-template"
                value={templateValue(draft.template_name, draft.template_language)}
                onChange={(event) => {
                  const [name, language] = event.target.value.split('\u0000')
                  change({
                    template_name: name,
                    template_language: language || draft.template_language,
                  })
                }}
                className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
              >
                <option value={templateValue('', draft.template_language)}>
                  Sin plantilla elegida
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
                Aún no tienes plantillas aprobadas. Crea una en Integraciones
                › Plantillas y espera a que Meta la apruebe.
              </p>
            )}

            {body && <p className="rounded-lg bg-muted p-3 text-xs">{body}</p>}

            {chosen && values !== REMINDER_PARAMETERS && (
              <p className="text-xs text-destructive">
                Un recordatorio rellena tres valores — el nombre, la fecha y la
                hora — pero esta plantilla admite {values}. Meta rechazará el
                envío.
              </p>
            )}

            {/*
              The variant picked here is no longer the language every
              reminder goes out in — each appointment carries its own. Said
              plainly because the consequence is not guessable from the
              control: a clinic that writes to patients in English needs the
              same template approved in en_US too, and the first sign that
              it is not would otherwise be a reminder Meta refused.
            */}
            {chosen && (
              <p className="text-xs text-muted-foreground">
                Es la variante por defecto. Cada cita lleva su propio idioma,
                así que necesitas esta plantilla aprobada en cada idioma al
                que le escribas.
              </p>
            )}

            {draft.template_name === '' && (
              <p className="text-xs text-muted-foreground">
                No se enviará ningún recordatorio hasta que elijas una plantilla.
                Fuera de una conversación, WhatsApp solo entrega plantillas
                aprobadas.
              </p>
            )}
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="flex items-center justify-end gap-3 pt-1">
            {saved && !dirty && <span className="text-sm text-muted-foreground">Guardado</span>}
            {dirty && (
              <Button variant="outline" onClick={() => setDraft(settings)} disabled={isSaving}>
                Descartar
              </Button>
            )}
            <Button onClick={save} disabled={isSaving || !dirty}>
              {isSaving ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Guardando...
                </>
              ) : (
                'Guardar'
              )}
            </Button>
          </div>
        </div>
  )
}

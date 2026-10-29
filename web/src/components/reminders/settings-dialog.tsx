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
import { TIMING_OPTIONS, type ReminderSettings } from '@/lib/reminder-settings'

/**
 * The two things a person decides about their reminders: how far ahead they
 * go out and whether they go out at all.
 *
 * The template and the zone are deliberately not here. They are stored (#9
 * sends with them) but there is nothing useful to choose from until #8 can
 * list the approved templates, and offering a free-text template name would
 * only produce sends that Meta rejects.
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

  useEffect(() => {
    if (open) {
      setDraft(settings)
      setError('')
    }
  }, [open, settings])

  const save = async () => {
    setIsSaving(true)
    try {
      await onSave(draft)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save your settings.')
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
            When a reminder goes out, and whether it goes out at all.
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

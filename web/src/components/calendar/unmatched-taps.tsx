import { AlertTriangle } from 'lucide-react'
import { useEffect, useState } from 'react'

import { spellDateTime } from '@/lib/dates'
import { listUnmatchedTaps, type Message } from '@/lib/messages'

/**
 * A patient pressed Confirmar or Cancelar under a reminder and the booking
 * did not move. Nowhere else in the app says so — the appointment looks
 * exactly as it did before the reminder went out — and a cancellation nobody
 * notices is worse than never having offered the button.
 *
 * It lives on the Agenda, where the fix is made, rather than among the
 * reminder settings: it is something to act on, not something to configure.
 */
export function UnmatchedTaps() {
  const [taps, setTaps] = useState<Message[]>([])

  useEffect(() => {
    // Swallowed: a warning that cannot be read is no reason to block the day.
    listUnmatchedTaps()
      .then(setTaps)
      .catch(() => setTaps([]))
  }, [])

  if (taps.length === 0) {
    return null
  }

  return (
    <div className="mb-6 flex items-start gap-3 rounded-[14px] bg-amber-50 p-4 ring-1 ring-amber-200 dark:bg-amber-500/10 dark:ring-amber-500/30">
      <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400" />
      <div className="min-w-0 space-y-1 text-sm text-amber-800 dark:text-amber-200">
        <p>
          <strong>Respuestas sin vincular.</strong> No pudimos saber a qué cita se referían.
          Cámbialas a mano en la agenda.
        </p>
        <ul className="space-y-0.5 text-xs">
          {taps.map((tap) => (
            <li key={tap.id}>
              +{tap.to_number} respondió «{tap.message_content}» el{' '}
              {spellDateTime(new Date(tap.sent_at))}
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}

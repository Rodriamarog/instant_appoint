import { useEffect, useState } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { errorMessage } from '@/lib/errors'
import { formatTime, listMessages, type Message } from '@/lib/messages'

/**
 * The thread an appointment was read out of — WhatsApp, Messenger or
 * Instagram.
 *
 * It exists because of one question. An appointment the assistant wrote —
 * and especially one it only proposed — provokes "why does this exist?", and
 * the only answer is the conversation. A link to a page elsewhere would be a
 * second question; this puts the answer where the question is asked.
 *
 * Read-only on purpose. Staff answer patients from the WhatsApp Business app
 * on their own phone, and a reply box here would be a second place to do it
 * with none of the same history.
 */
export function ConversationTranscript({ conversationId }: { conversationId: string }) {
  const [messages, setMessages] = useState<Message[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    // The dialog can be closed while this is in flight, and setting state on
    // a transcript nobody is looking at is how a closed dialog logs a
    // warning on every appointment somebody opens.
    let watching = true

    listMessages(conversationId)
      .then((rows) => {
        if (watching) setMessages(rows)
      })
      .catch((err) => {
        if (watching) setError(errorMessage(err, 'No se pudo cargar la conversación.'))
      })

    return () => {
      watching = false
    }
  }, [conversationId])

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    )
  }

  if (messages === null) {
    return <p className="text-sm text-muted-foreground">Cargando la conversación...</p>
  }

  if (messages.length === 0) {
    return <p className="text-sm text-muted-foreground">Esta conversación ya no tiene mensajes.</p>
  }

  // The patient may never have given a phone number on these two, which is
  // why the appointment shows none — worth saying where staff will look.
  const channel = CHANNEL_NAMES[messages[0].channel ?? 'whatsapp']

  return (
    <>
      {channel && (
        <p className="text-xs font-medium text-muted-foreground">Conversación por {channel}</p>
      )}
      <ul className="max-h-56 space-y-2 overflow-y-auto">
        {messages.map((message) => (
          <li key={message.id} className="text-sm">
            <span className="text-xs text-muted-foreground">
              {/*
              Who said it, not which direction it travelled. A receptionist
              reading this is working out who agreed to what, and "saliente"
              is the webhook's word, not theirs.
            */}
              {message.direction === 'inbound' ? 'Paciente' : 'Clínica'} ·{' '}
              {formatTime(message.sent_at)}
            </span>
            <p className="whitespace-pre-wrap">{message.message_content}</p>
          </li>
        ))}
      </ul>
    </>
  )
}

const CHANNEL_NAMES: Record<string, string | undefined> = {
  messenger: 'Messenger',
  instagram: 'Instagram',
}

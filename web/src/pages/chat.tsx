import { Loader2, MessageSquare, Send } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  formatPhone,
  liveConversations,
  type Conversation,
} from '@/lib/conversations'
import type { Feed } from '@/lib/feed'
import {
  formatTime,
  liveMessages,
  sendReply,
  withMessage,
  type Message,
  type MessageFeed,
} from '@/lib/messages'

/**
 * The WhatsApp inbox: the customers who have written, the thread with the one
 * being looked at, and a box to answer from.
 *
 * Nothing on this page is on a timer. Both lists are read through a feed —
 * see lib/feed.ts — which hands the page a fresh list whenever there may be
 * one, and the page does not know whether that came from the live connection
 * or from asking again. The two feeds arrive as props with live defaults, so
 * swapping either for polling is one argument, here or at the route, and no
 * change to anything below.
 */
export function ChatPage({
  conversations: conversationFeed = liveConversations,
  messages: messageFeed = liveMessages,
}: {
  conversations?: Feed<Conversation>
  messages?: MessageFeed
} = {}) {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const bottom = useRef<HTMLDivElement>(null)

  const describe = useCallback((error: unknown) => {
    setProblem(
      error instanceof Error ? error.message : 'Something went wrong. Try again.',
    )
  }, [])

  useEffect(
    () => conversationFeed(setConversations, describe),
    [conversationFeed, describe],
  )

  useEffect(() => {
    if (!selectedId) {
      setMessages([])

      return
    }

    // A thread from the conversation that was open a moment ago must not be
    // left on screen under the new one's name.
    setMessages([])

    return messageFeed(selectedId)(setMessages, describe)
  }, [selectedId, messageFeed, describe])

  useEffect(() => {
    // Optional call: jsdom has no scrollIntoView, and a thread that cannot
    // scroll itself is not a reason for the page to throw.
    bottom.current?.scrollIntoView?.({ behavior: 'smooth' })
  }, [messages])

  const selected = conversations.find((item) => item.id === selectedId) ?? null

  const send = async () => {
    const text = draft.trim()
    if (!text || !selectedId || sending) {
      return
    }

    setSending(true)
    setProblem(null)

    try {
      const sent = await sendReply(selectedId, text)
      setDraft('')
      // Shown straight away rather than waited for: the feed will deliver the
      // same row, and merging by id means it only appears once either way.
      setMessages((thread) => withMessage(thread, sent))
    } catch (error) {
      describe(error)
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="space-y-5">
      <h2 className="font-heading text-2xl font-bold">Chat</h2>

      {problem && (
        <p role="alert" className="text-sm text-destructive">
          {problem}
        </p>
      )}

      <div className="flex h-[70vh] overflow-hidden rounded-xl border">
        <div className="flex w-72 shrink-0 flex-col border-r">
          <div className="border-b bg-muted/40 px-4 py-3">
            <p className="text-sm font-medium">Conversations</p>
          </div>

          <ul className="flex-1 overflow-y-auto">
            {conversations.length === 0 && (
              <li className="mt-8 px-4 text-center text-sm text-muted-foreground">
                No conversations yet
              </li>
            )}

            {conversations.map((conversation) => (
              <li key={conversation.id}>
                <button
                  type="button"
                  aria-current={conversation.id === selectedId}
                  onClick={() => setSelectedId(conversation.id)}
                  className={`w-full border-b px-4 py-3 text-left transition-colors hover:bg-muted/60 ${
                    conversation.id === selectedId ? 'bg-muted' : ''
                  }`}
                >
                  <p className="text-sm font-medium">
                    {formatPhone(conversation.customer_phone)}
                  </p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {formatTime(conversation.last_message_at)}
                  </p>
                </button>
              </li>
            ))}
          </ul>
        </div>

        {!selected ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
            <MessageSquare className="h-5 w-5" />
            Select a conversation
          </div>
        ) : (
          <div className="flex flex-1 flex-col">
            <div className="border-b bg-muted/40 px-4 py-3">
              <p className="text-sm font-semibold">
                {formatPhone(selected.customer_phone)}
              </p>
            </div>

            <ol className="flex-1 space-y-2 overflow-y-auto px-4 py-4">
              {messages.map((message) => (
                <li
                  key={message.id}
                  data-direction={message.direction}
                  className={`flex ${
                    message.direction === 'outbound'
                      ? 'justify-end'
                      : 'justify-start'
                  }`}
                >
                  <div
                    className={`max-w-xs rounded-2xl px-3 py-2 text-sm lg:max-w-md ${
                      message.direction === 'outbound'
                        ? 'rounded-br-sm bg-primary text-primary-foreground'
                        : 'rounded-bl-sm bg-muted text-foreground'
                    }`}
                  >
                    <p className="whitespace-pre-wrap">{message.message_content}</p>
                    <p
                      className={`mt-1 text-xs ${
                        message.direction === 'outbound'
                          ? 'text-primary-foreground/70'
                          : 'text-muted-foreground'
                      }`}
                    >
                      {formatTime(message.sent_at)}
                    </p>
                  </div>
                </li>
              ))}
              <div ref={bottom} />
            </ol>

            <form
              className="flex gap-2 border-t px-4 py-3"
              onSubmit={(event) => {
                event.preventDefault()
                void send()
              }}
            >
              <Input
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                placeholder="Type a message..."
                aria-label="Message"
              />
              <Button type="submit" disabled={sending || !draft.trim()}>
                {sending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Send className="h-4 w-4" />
                )}
                {sending ? 'Sending' : 'Send'}
              </Button>
            </form>
          </div>
        )}
      </div>
    </div>
  )
}

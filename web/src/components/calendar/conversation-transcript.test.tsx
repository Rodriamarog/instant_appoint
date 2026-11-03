import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { ConversationTranscript } from '@/components/calendar/conversation-transcript'
import { pb } from '@/lib/pocketbase'

/**
 * The seam is the PocketBase SDK, as everywhere else in this app that reads
 * a collection straight from the browser.
 */
const getFullList = vi.fn()

beforeEach(() => {
  getFullList.mockReset().mockResolvedValue([])

  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList,
  } as unknown as ReturnType<typeof pb.collection>)
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function message(overrides: Record<string, unknown> = {}) {
  return {
    id: 'm1',
    direction: 'inbound',
    message_content: '¿Tienen lugar el viernes?',
    sent_at: '2026-11-02T17:00:00.000Z',
    ...overrides,
  }
}

test('each turn says who said it, in the words a receptionist uses', async () => {
  // "Entrante" and "saliente" are the webhook's words for which way a
  // message travelled. The person reading this is working out who agreed to
  // what, and the answer they need is a party, not a direction.
  getFullList.mockResolvedValue([
    message(),
    message({ id: 'm2', direction: 'outbound', message_content: 'Sí, a las 3.' }),
  ])

  render(<ConversationTranscript conversationId="conv1" />)

  expect((await screen.findByText(/Paciente/)).textContent).toBeDefined()
  expect(screen.getByText(/Clínica/)).toBeDefined()
  expect(screen.getByText('Sí, a las 3.')).toBeDefined()
})

test('only the thread the appointment came from is asked for', async () => {
  render(<ConversationTranscript conversationId="conv1" />)

  await screen.findByText(/no tiene mensajes/)
  expect(getFullList.mock.calls[0][0].filter).toContain('conv1')
})

test('a conversation with nothing left in it says so rather than looking empty', async () => {
  // An empty panel under a button somebody just pressed reads as broken.
  render(<ConversationTranscript conversationId="conv1" />)

  expect(await screen.findByText(/no tiene mensajes/)).toBeDefined()
})

test('a thread that cannot be read says so instead of showing nothing', async () => {
  // Silence here answers "why does this exist?" with the wrong answer: it
  // looks like the appointment came out of an empty conversation.
  getFullList.mockRejectedValue(new Error('The request failed.'))

  render(<ConversationTranscript conversationId="conv1" />)

  expect((await screen.findByRole('alert')).textContent).toMatch(/request failed/i)
})

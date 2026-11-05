import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { UnmatchedTaps } from '@/components/calendar/unmatched-taps'
import { pb } from '@/lib/pocketbase'

/**
 * A patient pressed a button under a reminder and the booking did not move.
 * Nothing else in the app will ever say so — the appointment looks exactly
 * as it did before the reminder went out — so the Agenda has to.
 */
const signedInUser = { id: 'user123', collectionId: '_pb_users_auth_', collectionName: 'users' }

let taps: Record<string, unknown>[] = []
const getFullList = vi.fn(() => Promise.resolve(taps))

beforeEach(() => {
  taps = []
  getFullList.mockClear()
  vi.spyOn(pb, 'collection').mockReturnValue({ getFullList } as unknown as ReturnType<
    typeof pb.collection
  >)
  pb.authStore.save('not-a-real-token', signedInUser)
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('a tap that matched no appointment is called out, with who and what', async () => {
  taps = [
    {
      id: 'msg1',
      user_id: signedInUser.id,
      conversation_id: 'conv1',
      direction: 'inbound',
      message_content: 'Cancelar',
      message_type: 'button',
      message_id: 'wamid.tap1',
      sent_at: '2026-11-02T11:30:00.000Z',
      to_number: '5215512345678',
      event_id: '',
      status: '',
    },
  ]

  render(<UnmatchedTaps />)

  const warning = await screen.findByText(/Respuestas sin vincular/)
  const banner = warning.closest('div')

  expect(banner?.textContent).toContain('5215512345678')
  expect(banner?.textContent).toContain('Cancelar')
})

test('nothing is said when every tap found its appointment', async () => {
  const { container } = render(<UnmatchedTaps />)

  await waitFor(() => expect(getFullList).toHaveBeenCalled())
  expect(container.textContent).toBe('')
})

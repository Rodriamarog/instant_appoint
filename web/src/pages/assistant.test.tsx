import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { pb } from '@/lib/pocketbase'
import { AssistantPage } from '@/pages/assistant'

/**
 * The seam is the PocketBase SDK, the same one lib/business-settings sits on.
 * Above it: what the page shows the clinic its assistant is doing, and what
 * reaches the row when they change it. The control's own behaviour — three
 * radios, which one is checked — belongs to
 * components/settings/ai-mode-field.test.tsx.
 */
const signedInUser = {
  id: 'user123',
  collectionId: '_pb_users_auth_',
  collectionName: 'users',
}

let storedSettings: Record<string, unknown> | null = null

const updateSettings = vi.fn()

beforeEach(() => {
  storedSettings = {
    id: 'set123',
    user_id: signedInUser.id,
    timing_minutes: 1440,
    is_active: true,
    template_name: 'recordatorio',
    template_language: 'es',
    timezone: 'America/Tijuana',
    ai_mode: 'off',
  }

  updateSettings.mockReset().mockImplementation((id, body) => {
    storedSettings = { ...storedSettings, id, ...body }

    return Promise.resolve(storedSettings)
  })

  vi.spyOn(pb, 'collection').mockReturnValue({
    getFirstListItem: () => Promise.resolve(storedSettings),
    update: updateSettings,
  } as unknown as ReturnType<typeof pb.collection>)

  pb.authStore.save('not-a-real-token', signedInUser)
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('the clinic is shown the position its assistant is actually in', async () => {
  storedSettings = { ...storedSettings, ai_mode: 'observer' }

  render(<AssistantPage />)

  const observer = await screen.findByRole('radio', { name: /Observador/ })
  expect(observer.getAttribute('aria-checked')).toBe('true')
})

/**
 * There is no Save button here, so the click is the save. The rest of the
 * row has to survive it: this page knows about one field and must not write
 * the other five back as defaults.
 */
test('choosing a position writes it down without disturbing the rest of the row', async () => {
  const user = userEvent.setup()

  render(<AssistantPage />)

  await user.click(await screen.findByRole('radio', { name: /Piloto automático/ }))

  await waitFor(() =>
    expect(updateSettings).toHaveBeenCalledWith(
      'set123',
      expect.objectContaining({
        ai_mode: 'autopilot',
        template_name: 'recordatorio',
        timing_minutes: 1440,
      }),
    ),
  )
})

/**
 * A control that moves and stays moved while nothing was written is how a
 * clinic comes to believe its patients are being answered when they are not.
 */
test('a save that fails puts the switch back and says so', async () => {
  const user = userEvent.setup()
  updateSettings.mockRejectedValue(new Error('La red falló.'))

  render(<AssistantPage />)

  await user.click(await screen.findByRole('radio', { name: /Piloto automático/ }))

  expect(await screen.findByText('La red falló.')).toBeDefined()
  await waitFor(() =>
    expect(
      screen.getByRole('radio', { name: /Apagado/ }).getAttribute('aria-checked'),
    ).toBe('true'),
  )
})

import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { EventDialog } from '@/components/calendar/event-dialog'
import type { CalendarEvent } from '@/lib/calendar-events'
import { pb } from '@/lib/pocketbase'

/**
 * The seam is the PocketBase SDK: everything above it — the form, the
 * generated reminder text, the validation — is the behaviour under test, and
 * what lands in the collection is how that behaviour is observed. The network
 * itself is the only thing stubbed.
 */
const create = vi.fn()
const update = vi.fn()
const remove = vi.fn()

const signedInUser = {
  id: 'user123',
  collectionId: '_pb_users_auth_',
  collectionName: 'users',
  business_name: 'Dev Clinic',
}

/** The current value of a form control found by label. */
function value(label: RegExp | string): string {
  const field = screen.getByLabelText(label)

  return (field as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement).value
}

beforeEach(() => {
  create.mockReset().mockResolvedValue({ id: 'new123' })
  update.mockReset().mockResolvedValue({ id: 'evt123' })
  remove.mockReset().mockResolvedValue(true)

  vi.spyOn(pb, 'collection').mockReturnValue({
    create,
    update,
    delete: remove,
  } as unknown as ReturnType<typeof pb.collection>)

  // A real session, because the dialog reads both the owner id it has to
  // stamp on a create and the business name the reminder copy uses.
  pb.authStore.save('not-a-real-token', signedInUser)

  // The native confirm() is not implemented in jsdom, and the question it
  // asks is not what this suite is about.
  vi.stubGlobal('confirm', vi.fn().mockReturnValue(true))
})

afterEach(() => {
  cleanup()
  pb.authStore.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

/** An appointment as PocketBase returns it, with a reminder the owner wrote. */
function savedEvent(overrides: Partial<CalendarEvent> = {}): CalendarEvent {
  return {
    id: 'evt123',
    user_id: signedInUser.id,
    title: 'Ana Ruiz',
    client_phone: '5215512345678',
    notes: 'See you at the back entrance, Ana.',
    start_time: '2026-11-02T17:00:00.000Z',
    end_time: '2026-11-02T18:00:00.000Z',
    location: 'Suite 2',
    ...overrides,
  }
}

function renderDialog(props: Partial<ComponentProps<typeof EventDialog>> = {}) {
  const onSaved = vi.fn()
  const onClose = vi.fn()

  render(
    <EventDialog
      open
      mode="edit"
      event={savedEvent()}
      onClose={onClose}
      onSaved={onSaved}
      {...props}
    />,
  )

  return { onSaved, onClose }
}

/** A blank slot, as selecting a range on the calendar produces one. */
const newSlot: CalendarEvent = {
  id: '',
  user_id: '',
  title: '',
  start_time: '2026-11-09T17:00:00.000Z',
  end_time: '2026-11-09T18:00:00.000Z',
}

test('a saved reminder message survives opening the appointment and pressing Save', async () => {
  // The whole bug: the generator used to run on mount for an existing event,
  // so just looking at an appointment and saving rewrote what its owner had
  // written.
  const user = userEvent.setup()
  renderDialog()

  expect(value(/reminder message/i)).toBe('See you at the back entrance, Ana.')

  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][1]).toMatchObject({
    notes: 'See you at the back entrance, Ana.',
  })
})

test('a hand-edited reminder message survives a change to the client name', async () => {
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  const reminder = screen.getByLabelText(/reminder message/i)
  await user.clear(reminder)
  await user.type(reminder, 'Bring your x-rays.')

  await user.type(screen.getByLabelText(/client name/i), 'Ana')

  expect(value(/reminder message/i)).toBe('Bring your x-rays.')

  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(create).toHaveBeenCalled())
  expect(create.mock.calls[0][0]).toMatchObject({ notes: 'Bring your x-rays.' })
})

test('an untouched reminder message still follows the client name', async () => {
  // The generator is the feature; only overwriting someone's words was the
  // bug. On a fresh appointment nobody has typed into, it still fills in.
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/client name/i), 'Ana')

  expect(value(/reminder message/i)).toContain('Ana')
  expect(value(/reminder message/i)).toContain('Dev Clinic')
})

test('a new appointment is created for the signed-in person', async () => {
  const user = userEvent.setup()
  const { onSaved } = renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/client name/i), 'Ana Ruiz')
  await user.type(screen.getByLabelText(/phone number/i), '5512345678')
  await user.type(screen.getByLabelText(/location/i), 'Suite 2')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(create).toHaveBeenCalled())

  const body = create.mock.calls[0][0]
  // The collection's create rule compares user_id against the caller's id, so
  // a create that omits it is rejected outright.
  expect(body.user_id).toBe(signedInUser.id)
  expect(body.title).toBe('Ana Ruiz')
  expect(body.location).toBe('Suite 2')
  expect(onSaved).toHaveBeenCalled()
})

test('the country code is stored as a prefix on the phone number', async () => {
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/client name/i), 'Ana Ruiz')
  await user.selectOptions(screen.getByLabelText(/country code/i), '521')
  await user.type(screen.getByLabelText(/phone number/i), '55 1234 5678')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(create).toHaveBeenCalled())
  expect(create.mock.calls[0][0].client_phone).toBe('5215512345678')
})

test('a stored phone number is split back into its country code and digits', () => {
  renderDialog({ event: savedEvent({ client_phone: '5215512345678' }) })

  expect(value(/country code/i)).toBe('521')
  expect(value(/phone number/i)).toBe('5512345678')
})

test('the times written are the millisecond-precision UTC format', async () => {
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/client name/i), 'Ana Ruiz')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(create).toHaveBeenCalled())

  const body = create.mock.calls[0][0]
  // A text column compared byte by byte: anything but this exact shape sorts
  // wrongly against the rows already stored.
  const shape = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/
  expect(body.start_time).toMatch(shape)
  expect(body.end_time).toMatch(shape)
})

test('an appointment opened for editing keeps the time it was saved with', async () => {
  // Reading the date in UTC and the time in local used to shift an evening
  // appointment by a day the moment it was reopened and saved.
  const user = userEvent.setup()
  const saved = savedEvent({
    start_time: new Date(2026, 10, 2, 23, 30).toISOString(),
    end_time: new Date(2026, 10, 3, 0, 30).toISOString(),
  })
  renderDialog({ event: saved })

  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][1]).toMatchObject({
    start_time: saved.start_time,
    end_time: saved.end_time,
  })
})

test('an appointment that ends before it starts is refused', async () => {
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/client name/i), 'Ana Ruiz')
  await user.clear(screen.getByLabelText(/end time/i))
  await user.type(screen.getByLabelText(/end time/i), '08:00')
  await user.clear(screen.getByLabelText(/start time/i))
  await user.type(screen.getByLabelText(/start time/i), '09:00')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect((await screen.findByRole('alert')).textContent).toMatch(/end time/i)
  expect(create).not.toHaveBeenCalled()
})

test('an appointment with no client name is refused', async () => {
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect((await screen.findByRole('alert')).textContent).toMatch(/client name/i)
  expect(create).not.toHaveBeenCalled()
})

test('editing an existing appointment updates it rather than creating another', async () => {
  const user = userEvent.setup()
  const { onSaved } = renderDialog()

  const name = screen.getByLabelText(/client name/i)
  await user.clear(name)
  await user.type(name, 'Ana Ruiz-Moreno')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][0]).toBe('evt123')
  expect(update.mock.calls[0][1]).toMatchObject({ title: 'Ana Ruiz-Moreno' })
  expect(create).not.toHaveBeenCalled()
  expect(onSaved).toHaveBeenCalled()
})

test('an edit keeps the location rather than erasing it', async () => {
  // The route this replaces dropped location from its PATCH body, so every
  // edit silently cleared it.
  const user = userEvent.setup()
  renderDialog()

  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][1]).toMatchObject({ location: 'Suite 2' })
})

test('an appointment is deleted from the dialog', async () => {
  const user = userEvent.setup()
  const { onSaved } = renderDialog()

  await user.click(screen.getByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(remove).toHaveBeenCalledWith('evt123'))
  expect(onSaved).toHaveBeenCalled()
})

test('a new appointment offers no Delete button', () => {
  renderDialog({ mode: 'create', event: newSlot })

  expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull()
})

test('a rejected save is reported rather than swallowed', async () => {
  const user = userEvent.setup()
  create.mockRejectedValue(new Error('Failed to create record.'))
  const { onSaved } = renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/client name/i), 'Ana Ruiz')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  expect((await screen.findByRole('alert')).textContent).toMatch(
    /failed to create record/i,
  )
  expect(onSaved).not.toHaveBeenCalled()
})

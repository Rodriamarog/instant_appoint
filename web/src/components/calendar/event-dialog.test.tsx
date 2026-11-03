import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { EventDialog } from '@/components/calendar/event-dialog'
import type { Appointment } from '@/lib/appointments'
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
const getFullList = vi.fn()

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
  getFullList.mockReset().mockResolvedValue([])

  vi.spyOn(pb, 'collection').mockReturnValue({
    create,
    update,
    delete: remove,
    getFullList,
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
function savedEvent(overrides: Partial<Appointment> = {}): Appointment {
  return {
    id: 'evt123',
    user_id: signedInUser.id,
    client_name: 'Ana Ruiz',
    client_phone: '5215512345678',
    notes: 'See you at the back entrance, Ana.',
    start_time: '2026-11-02T17:00:00.000Z',
    end_time: '2026-11-02T18:00:00.000Z',
    location: 'Suite 2',
    status: 'pending',
    status_set_by: 'staff',
    // Built off a local clock, so the moment the dialog prints is the same
    // wherever this suite runs.
    status_set_at: new Date(2026, 10, 2, 9, 30).toISOString(),
    created_by: 'staff',
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
const newSlot: Appointment = {
  id: '',
  user_id: '',
  client_name: '',
  start_time: '2026-11-09T17:00:00.000Z',
  end_time: '2026-11-09T18:00:00.000Z',
  status: 'pending',
}

test('the dialog offers no reminder message to write', () => {
  // Reminders go out well outside Meta's 24-hour window, where only an
  // approved template is accepted, so prose written per appointment was
  // undeliverable and nothing ever read it. The control's only effect was to
  // teach staff that typing here changed what the client received.
  renderDialog({ mode: 'create', event: newSlot })

  expect(screen.queryByLabelText(/mensaje de recordatorio/i)).toBeNull()
})

test('a note is the staff member’s own words, never copy written for them', async () => {
  // The control this replaces filled itself in from the client name and the
  // business name, and rewrote itself on every keystroke in the name field.
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/nombre del cliente/i), 'Ana')

  expect(value(/notas/i)).toBe('')

  await user.type(screen.getByLabelText(/notas/i), 'Bring your x-rays.')

  expect(value(/notas/i)).toBe('Bring your x-rays.')

  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(create).toHaveBeenCalled())
  expect(create.mock.calls[0][0]).toMatchObject({ notes: 'Bring your x-rays.' })
})

test('text written when the field was a reminder message is still shown and still kept', async () => {
  // Only the control went; the column stayed. A row written under the old
  // label has to come back verbatim and survive a save, or removing the
  // control would quietly erase what people had typed.
  const user = userEvent.setup()
  renderDialog()

  expect(value(/notas/i)).toBe('See you at the back entrance, Ana.')

  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][1]).toMatchObject({
    notes: 'See you at the back entrance, Ana.',
  })
})

test('a note changes nothing about what a reminder is built from', async () => {
  // A reminder is an approved template filled with the client's name, the
  // date and the time, read off client_name, client_phone and start_time.
  // Those three are the whole of an appointment's contribution to one, so
  // they are what has to be unmoved by dropping the message control.
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/nombre del cliente/i), 'Ana Ruiz')
  await user.selectOptions(screen.getByLabelText(/código de país/i), '521')
  await user.type(screen.getByLabelText(/número de teléfono/i), '5512345678')
  await user.type(screen.getByLabelText(/notas/i), 'Allergic to latex.')
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(create).toHaveBeenCalled())

  const body = create.mock.calls[0][0]
  expect(body.client_name).toBe('Ana Ruiz')
  expect(body.client_phone).toBe('5215512345678')
  expect(body.start_time).toBe(newSlot.start_time)
})

test('a new appointment is created for the signed-in person', async () => {
  const user = userEvent.setup()
  const { onSaved } = renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/nombre del cliente/i), 'Ana Ruiz')
  await user.type(screen.getByLabelText(/número de teléfono/i), '5512345678')
  await user.type(screen.getByLabelText(/lugar/i), 'Suite 2')
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(create).toHaveBeenCalled())

  const body = create.mock.calls[0][0]
  // The collection's create rule compares user_id against the caller's id, so
  // a create that omits it is rejected outright.
  expect(body.user_id).toBe(signedInUser.id)
  expect(body.client_name).toBe('Ana Ruiz')
  expect(body.location).toBe('Suite 2')
  expect(onSaved).toHaveBeenCalled()
})

test('the country code is stored as a prefix on the phone number', async () => {
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/nombre del cliente/i), 'Ana Ruiz')
  await user.selectOptions(screen.getByLabelText(/código de país/i), '521')
  await user.type(screen.getByLabelText(/número de teléfono/i), '55 1234 5678')
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(create).toHaveBeenCalled())
  expect(create.mock.calls[0][0].client_phone).toBe('5215512345678')
})

test('a stored phone number is split back into its country code and digits', () => {
  renderDialog({ event: savedEvent({ client_phone: '5215512345678' }) })

  expect(value(/código de país/i)).toBe('521')
  expect(value(/número de teléfono/i)).toBe('5512345678')
})

test('the times written are the millisecond-precision UTC format', async () => {
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/nombre del cliente/i), 'Ana Ruiz')
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

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

  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][1]).toMatchObject({
    start_time: saved.start_time,
    end_time: saved.end_time,
  })
})

test('an appointment that ends before it starts is refused', async () => {
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/nombre del cliente/i), 'Ana Ruiz')
  await user.clear(screen.getByLabelText(/hora de fin/i))
  await user.type(screen.getByLabelText(/hora de fin/i), '08:00')
  await user.clear(screen.getByLabelText(/hora de inicio/i))
  await user.type(screen.getByLabelText(/hora de inicio/i), '09:00')
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  expect((await screen.findByRole('alert')).textContent).toMatch(/hora de fin/i)
  expect(create).not.toHaveBeenCalled()
})

test('an appointment with no client name is refused', async () => {
  const user = userEvent.setup()
  renderDialog({ mode: 'create', event: newSlot })

  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  expect((await screen.findByRole('alert')).textContent).toMatch(/nombre del cliente/i)
  expect(create).not.toHaveBeenCalled()
})

test('editing an existing appointment updates it rather than creating another', async () => {
  const user = userEvent.setup()
  const { onSaved } = renderDialog()

  const name = screen.getByLabelText(/nombre del cliente/i)
  await user.clear(name)
  await user.type(name, 'Ana Ruiz-Moreno')
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][0]).toBe('evt123')
  expect(update.mock.calls[0][1]).toMatchObject({ client_name: 'Ana Ruiz-Moreno' })
  expect(create).not.toHaveBeenCalled()
  expect(onSaved).toHaveBeenCalled()
})

test('an edit keeps the location rather than erasing it', async () => {
  // The route this replaces dropped location from its PATCH body, so every
  // edit silently cleared it.
  const user = userEvent.setup()
  renderDialog()

  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][1]).toMatchObject({ location: 'Suite 2' })
})

test('an appointment is deleted from the dialog', async () => {
  const user = userEvent.setup()
  const { onSaved } = renderDialog()

  await user.click(screen.getByRole('button', { name: 'Eliminar' }))

  await waitFor(() => expect(remove).toHaveBeenCalledWith('evt123'))
  expect(onSaved).toHaveBeenCalled()
})

test('a new appointment offers no Delete button', () => {
  renderDialog({ mode: 'create', event: newSlot })

  expect(screen.queryByRole('button', { name: 'Eliminar' })).toBeNull()
})

// --- status and provenance ------------------------------------------------

test('an appointment opened for editing says where it stands, who put it there and when', async () => {
  // "Who said so" is the first thing asked of a screen that says cancelled,
  // and the three possible answers mean very different things: staff
  // cancelling is a decision, the patient cancelling is news.
  renderDialog({
    event: savedEvent({
      status: 'cancelled',
      status_set_by: 'patient',
      status_set_at: new Date(2026, 10, 2, 9, 30).toISOString(),
    }),
  })

  const shown = await screen.findByText(/Cancelada/)
  expect(shown.textContent).toMatch(/por el paciente/)
  expect(shown.textContent).toContain('lun 2 nov 2026, 09:30')
})

test('an appointment the assistant only proposed says so rather than looking booked', async () => {
  // A proposal is the assistant's reading of a conversation, not a
  // commitment. Showing it like any other appointment is how a clinic ends
  // up holding a slot nobody agreed to.
  renderDialog({
    event: savedEvent({ status: 'proposed', status_set_by: 'assistant' }),
  })

  const shown = await screen.findByText(/Propuesta/)
  expect(shown.textContent).toMatch(/por el asistente/)
})

test('staff can confirm an appointment from the dialog', async () => {
  const user = userEvent.setup()
  const { onSaved } = renderDialog()

  await user.click(screen.getByRole('button', { name: 'Marcar confirmada' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][0]).toBe('evt123')
  expect(update.mock.calls[0][1]).toMatchObject({
    status: 'confirmed',
    status_set_by: 'staff',
  })
  expect(onSaved).toHaveBeenCalled()
})

test('staff can cancel an appointment from the dialog', async () => {
  // Cancelling is not deleting. The booking, the phone number and the
  // history stay; what changes is that nobody is coming, and no reminder
  // goes out.
  const user = userEvent.setup()
  renderDialog()

  await user.click(screen.getByRole('button', { name: 'Marcar cancelada' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][1]).toMatchObject({
    status: 'cancelled',
    status_set_by: 'staff',
  })
  expect(remove).not.toHaveBeenCalled()
})

test('a status change carries nothing but the status and who set it', async () => {
  // Deciding something about the appointment is not editing it. A status
  // button that posted the whole form would save half-typed changes the
  // person had not pressed Save on.
  const user = userEvent.setup()
  renderDialog()

  await user.type(screen.getByLabelText(/nombre del cliente/i), ' (escrito sin guardar)')
  await user.click(screen.getByRole('button', { name: 'Marcar confirmada' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(Object.keys(update.mock.calls[0][1]).sort()).toEqual([
    'status',
    'status_set_at',
    'status_set_by',
  ])
})

test('an appointment already cancelled is not offered a second cancel', async () => {
  renderDialog({ event: savedEvent({ status: 'cancelled' }) })

  expect(screen.queryByRole('button', { name: 'Marcar cancelada' })).toBeNull()
  expect(screen.getByRole('button', { name: 'Marcar confirmada' })).toBeDefined()
})

test('a new appointment has no status to show or set yet', () => {
  // It is not an appointment until it is saved, and it will be pending when
  // it is. Offering Confirm on something that does not exist would write a
  // status against an empty id.
  renderDialog({ mode: 'create', event: newSlot })

  expect(screen.queryByRole('button', { name: 'Marcar confirmada' })).toBeNull()
  expect(screen.queryByText(/Estado/)).toBeNull()
})

test('a rejected status change is reported rather than swallowed', async () => {
  const user = userEvent.setup()
  update.mockRejectedValue(new Error('Failed to update record.'))
  const { onSaved } = renderDialog()

  await user.click(screen.getByRole('button', { name: 'Marcar confirmada' }))

  expect((await screen.findByRole('alert')).textContent).toMatch(
    /failed to update record/i,
  )
  expect(onSaved).not.toHaveBeenCalled()
})

test('a rejected save is reported rather than swallowed', async () => {
  const user = userEvent.setup()
  create.mockRejectedValue(new Error('Failed to create record.'))
  const { onSaved } = renderDialog({ mode: 'create', event: newSlot })

  await user.type(screen.getByLabelText(/nombre del cliente/i), 'Ana Ruiz')
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  expect((await screen.findByRole('alert')).textContent).toMatch(
    /failed to create record/i,
  )
  expect(onSaved).not.toHaveBeenCalled()
})

// --- proposals and the conversation behind them ---------------------------

test('a proposal becomes a real appointment in one action', async () => {
  // The proposal is the assistant saying it read an agreement but is not
  // certain. Turning it into a booking is one person deciding one thing, so
  // it is one button — not "edit the appointment, change the status, save".
  const user = userEvent.setup()
  const { onSaved } = renderDialog({
    event: savedEvent({ status: 'proposed', status_set_by: 'assistant', created_by: 'assistant' }),
  })

  await user.click(screen.getByRole('button', { name: 'Aceptar la cita' }))

  await waitFor(() => expect(update).toHaveBeenCalled())
  expect(update.mock.calls[0][0]).toBe('evt123')
  expect(update.mock.calls[0][1]).toMatchObject({
    // Pending, not confirmed: accepting a proposal says the appointment is
    // real, and nobody has asked the patient anything yet.
    status: 'pending',
    status_set_by: 'staff',
  })
  expect(onSaved).toHaveBeenCalled()
})

test('an appointment that is already real is not offered the accept action', () => {
  renderDialog({ event: savedEvent({ status: 'pending' }) })

  expect(screen.queryByRole('button', { name: 'Aceptar la cita' })).toBeNull()
})

test('the conversation an appointment came out of is one click away', async () => {
  // "Why does this exist?" is the question a proposal provokes, and the only
  // answer is the thread it was read out of. A link to a page elsewhere
  // would be a second question; this is the answer in place.
  const user = userEvent.setup()
  getFullList.mockResolvedValue([
    { id: 'm1', direction: 'inbound', message_content: '¿Tienen lugar el viernes?', sent_at: '2026-11-02T17:00:00.000Z' },
    { id: 'm2', direction: 'outbound', message_content: 'Sí, a las 3 de la tarde.', sent_at: '2026-11-02T17:01:00.000Z' },
  ])
  renderDialog({
    event: savedEvent({ status: 'proposed', created_by: 'assistant', conversation_id: 'conv1' }),
  })

  await user.click(screen.getByRole('button', { name: /ver la conversación/i }))

  expect(await screen.findByText('¿Tienen lugar el viernes?')).toBeDefined()
  expect(screen.getByText('Sí, a las 3 de la tarde.')).toBeDefined()
})

test('an appointment booked at the desk has no conversation to offer', () => {
  renderDialog({ event: savedEvent({ conversation_id: '' }) })

  expect(screen.queryByRole('button', { name: /ver la conversación/i })).toBeNull()
})

import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { AIModeField } from '@/components/settings/ai-mode-field'
import { AI_MODE_OPTIONS, type AIMode } from '@/lib/business-settings'

/**
 * The seam is the control's two props: the mode it is showing and the mode it
 * reports when somebody picks one. The page above it owns saving, so none of
 * that is here.
 *
 * This is the whole of the switch's UI, in its own file rather than inside
 * the reminder settings dialog, so that the Configuración page can take it
 * over by moving one line.
 */
const onChange = vi.fn()

function show(value: AIMode) {
  onChange.mockClear()
  render(<AIModeField value={value} onChange={onChange} />)
}

afterEach(cleanup)

test('all three positions are offered', () => {
  show('off')

  for (const option of AI_MODE_OPTIONS) {
    expect(screen.getByRole('radio', { name: new RegExp(option.label) })).toBeDefined()
  }
})

test('each position shows, side by side, whether it answers and whether it books', () => {
  show('off')

  // The labels are three words each and the difference between two of them is
  // not guessable from the words alone; without this the clinic is choosing
  // what its patients are told by reading a thesaurus. Every card carries
  // the same checklist, so the comparison is made at a glance.
  const card = (label: RegExp) => screen.getByRole('radio', { name: label })

  for (const label of [/Apagado/, /Observador/, /Piloto automático/]) {
    expect(card(label).textContent).toMatch(/Contesta a tus pacientes/)
    expect(card(label).textContent).toMatch(/Agenda las citas acordadas/)
  }
  expect(within(card(/Observador/)).getAllByLabelText('No')).toHaveLength(1)
  expect(within(card(/Piloto automático/)).queryAllByLabelText('No')).toHaveLength(0)
  expect(within(card(/Apagado/)).getAllByLabelText('No')).toHaveLength(2)
})

test('the chosen position is spelled out in full', () => {
  show('observer')

  expect(screen.getByText(/Observa las conversaciones/i)).toBeDefined()
  expect(screen.queryByText(/responde solo, por WhatsApp/i)).toBeNull()
})

test('both active positions say that they write appointments', () => {
  // The two of them put rows on the clinic's calendar, and a clinic that
  // switched to Observador expecting a silent bystander would find
  // appointments it never typed. Whether the assistant talks to the patient
  // and whether it writes are two different promises, and only one of them
  // is in the name of the position.
  show('off')

  for (const option of AI_MODE_OPTIONS.filter((o) => o.mode !== 'off')) {
    expect(option.help).toMatch(/cita/i)
  }
})

test('both active positions say the patient is asked to confirm', () => {
  // Observador promises the assistant does not answer the patient's
  // messages, and it still does not — but it now sends one message of its
  // own the moment it writes an appointment, and a clinic reading only the
  // name of the position would not expect that. The promise a patient
  // experiences has changed, so the words a clinic chooses by have to.
  show('off')

  for (const option of AI_MODE_OPTIONS.filter((o) => o.mode !== 'off')) {
    expect(option.help).toMatch(/confirm/i)
  }
})

test('the saved position is the one shown as chosen', () => {
  show('observer')

  expect(screen.getByRole('radio', { name: /Observador/ }).getAttribute('aria-checked')).toBe(
    'true',
  )
  expect(screen.getByRole('radio', { name: /Apagado/ }).getAttribute('aria-checked')).toBe(
    'false',
  )
})

test('picking a position reports it', async () => {
  const user = userEvent.setup()
  show('off')

  await user.click(screen.getByRole('radio', { name: /Piloto automático/ }))

  expect(onChange).toHaveBeenCalledWith('autopilot')
})

// A row written before the switch existed, or by a newer deploy that knows a
// fourth position, reaches this control as a value it has no button for.
// Showing one of the three as chosen anyway would tell the clinic its
// assistant is in a state it is not in.
test('a stored value this build does not know shows nothing as chosen', () => {
  show('supervised' as AIMode)

  for (const option of AI_MODE_OPTIONS) {
    expect(
      screen.getByRole('radio', { name: new RegExp(option.label) }).getAttribute('aria-checked'),
    ).toBe('false')
  }
})

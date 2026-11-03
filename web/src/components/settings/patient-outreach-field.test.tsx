import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { PatientOutreachField } from '@/components/settings/patient-outreach-field'
import { type PatientOutreach } from '@/lib/business-settings'

/**
 * The seam is the control's two props: the stored answer it is showing and
 * the answer it reports when somebody flips it. The page above it owns
 * saving, so none of that is here.
 */
const onChange = vi.fn()

function show(value: PatientOutreach) {
  onChange.mockClear()
  render(<PatientOutreachField value={value} onChange={onChange} />)
}

afterEach(cleanup)

test('it says which message it is about', () => {
  // "Mensajes que el asistente manda por su cuenta" is not a thing a clinic
  // can picture. The one message it governs today is the receipt, and the
  // clinic is deciding whether its patients get it.
  show('on')

  expect(screen.getByText(/agenda una cita/i)).toBeDefined()
  expect(screen.getByText(/confirme o lo cancele/i)).toBeDefined()
})

test('it says what turning it off does not touch', () => {
  // Reminders and autopilot replies are the other two ways a patient hears
  // from this app, and both keep going. A clinic that read this control as
  // "stop messaging my patients" and switched it on for that reason would
  // be making a decision it did not make.
  show('on')

  expect(screen.getByText(/no apaga tus recordatorios/i)).toBeDefined()
  expect(screen.getByText(/piloto automático/i)).toBeDefined()
})

test('the stored answer is the one shown', () => {
  show('on')
  expect(screen.getByRole('switch').getAttribute('aria-checked')).toBe('true')

  cleanup()

  show('off')
  expect(screen.getByRole('switch').getAttribute('aria-checked')).toBe('false')
})

test('turning it off reports off, and turning it back on reports on', async () => {
  const user = userEvent.setup()

  show('on')
  await user.click(screen.getByRole('switch'))
  expect(onChange).toHaveBeenCalledWith('off')

  cleanup()

  show('off')
  await user.click(screen.getByRole('switch'))
  expect(onChange).toHaveBeenCalledWith('on')
})

test('a row that predates this column shows the receipt as being sent', () => {
  // Which it is: internal/webhook treats anything but a deliberate "off" as
  // permission, so a clinic shown a switch in the off position would be
  // told its patients hear nothing while they are still getting the
  // receipt. Flipping it from here then has to produce a real "off".
  show('' as PatientOutreach)

  expect(screen.getByRole('switch').getAttribute('aria-checked')).toBe('true')
})

test('an answer this build does not know is read the way the backend reads it', async () => {
  const user = userEvent.setup()
  show('disabled' as PatientOutreach)

  expect(screen.getByRole('switch').getAttribute('aria-checked')).toBe('true')

  await user.click(screen.getByRole('switch'))
  expect(onChange).toHaveBeenCalledWith('off')
})

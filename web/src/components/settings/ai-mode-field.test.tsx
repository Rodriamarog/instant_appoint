import { cleanup, render, screen } from '@testing-library/react'
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

test('each position says in plain words what it does', () => {
  show('off')

  // The labels are three words each and the difference between two of them is
  // not guessable from the words alone; without this the clinic is choosing
  // what its patients are told by reading a thesaurus.
  expect(screen.getByText(/no responde ni escribe nada/i)).toBeDefined()
  expect(screen.getByText(/Solo observa las conversaciones/i)).toBeDefined()
  expect(screen.getByText(/responde solo, por WhatsApp/i)).toBeDefined()
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

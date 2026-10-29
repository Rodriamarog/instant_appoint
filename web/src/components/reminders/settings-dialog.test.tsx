import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { ReminderSettingsDialog } from '@/components/reminders/settings-dialog'
import { DEFAULT_SETTINGS, type ReminderSettings } from '@/lib/reminder-settings'

/**
 * The seam is the templates endpoint, stubbed, plus the onSave the page
 * passes in. Everything above it is the behaviour under test: that a template
 * can be chosen at all, that choosing one carries its language, and that a
 * template which does not fit a reminder is called out before it is saved.
 *
 * Until this existed, template_name had no default and nothing could set it,
 * so the scheduler had nothing to send and would have skipped every
 * appointment forever.
 */
const approved = {
  name: 'appointment_reminder',
  status: 'APPROVED',
  category: 'UTILITY',
  language: 'es',
  components: [
    { type: 'BODY', text: 'Hola {{1}}, te esperamos el {{2}} a las {{3}}.' },
  ],
}

const twoValues = {
  name: 'short_reminder',
  status: 'APPROVED',
  category: 'UTILITY',
  language: 'en_US',
  components: [{ type: 'BODY', text: 'Hi {{1}}, see you {{2}}.' }],
}

const pending = {
  name: 'not_yet_approved',
  status: 'PENDING',
  category: 'UTILITY',
  language: 'es',
  components: [{ type: 'BODY', text: 'Hola {{1}} {{2}} {{3}}.' }],
}

function stubTemplates(templates: unknown[], ok = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok,
        json: () =>
          Promise.resolve(ok ? { templates } : { message: 'No WhatsApp number connected.' }),
      } as Response),
    ),
  )
}

const onSave = vi.fn(() => Promise.resolve())

function show(settings: Partial<ReminderSettings> = {}) {
  render(
    <ReminderSettingsDialog
      open
      onOpenChange={() => {}}
      settings={{ ...DEFAULT_SETTINGS, ...settings }}
      onSave={onSave}
    />,
  )
}

beforeEach(() => {
  onSave.mockClear()
  stubTemplates([approved, twoValues, pending])
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

test('an approved template can be chosen and is saved with its language', async () => {
  const user = userEvent.setup()
  show()

  const picker = await screen.findByLabelText('Approved template')
  // An English template, so the language has to travel with the name rather
  // than being left at the Spanish default: Meta refuses the wrong pair.
  await user.selectOptions(picker, 'short_reminder (en_US)')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() =>
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        template_name: 'short_reminder',
        template_language: 'en_US',
      }),
    ),
  )
})

test('a template Meta has not approved is not offered', async () => {
  show()

  await screen.findByLabelText('Approved template')
  expect(screen.queryByRole('option', { name: /not_yet_approved/ })).toBeNull()
  expect(screen.getByRole('option', { name: /appointment_reminder/ })).toBeDefined()
})

test('nothing chosen yet says so rather than looking configured', async () => {
  show({ template_name: '' })

  expect(
    await screen.findByText(/Reminders will not send until you choose a template/),
  ).toBeDefined()
})

test('the chosen template shows the words the customer will read', async () => {
  show({ template_name: 'appointment_reminder', template_language: 'es' })

  expect(
    await screen.findByText('Hola {{1}}, te esperamos el {{2}} a las {{3}}.'),
  ).toBeDefined()
})

test('a template that does not take three values is called out', async () => {
  const user = userEvent.setup()
  show()

  const picker = await screen.findByLabelText('Approved template')
  await user.selectOptions(picker, 'short_reminder (en_US)')

  // A reminder always sends the name, the date and the time, so a two-value
  // template is a send Meta will refuse.
  expect(await screen.findByText(/this template takes 2/)).toBeDefined()
})

test('an account with no approved templates is told where to make one', async () => {
  stubTemplates([pending])
  show()

  expect(await screen.findByText(/no approved templates yet/)).toBeDefined()
})

test('a templates call that fails leaves the rest of the dialog usable', async () => {
  const user = userEvent.setup()
  stubTemplates([], false)
  show()

  expect(await screen.findByText(/Could not load your approved templates/)).toBeDefined()

  await user.click(screen.getByRole('button', { name: '30 min' }))
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() =>
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ timing_minutes: 30 })),
  )
})

test('a template already saved is kept even when the list cannot be read', async () => {
  stubTemplates([], false)
  show({ template_name: 'appointment_reminder', template_language: 'es' })

  const picker = await screen.findByLabelText('Approved template')
  expect((picker as HTMLSelectElement).selectedOptions[0].textContent).toContain(
    'appointment_reminder',
  )
})

test('the zone the date and time are spelled in can be chosen', async () => {
  const user = userEvent.setup()
  show()

  const picker = await screen.findByLabelText('Spell the date and time in')
  await user.selectOptions(picker, 'America/Mexico City')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() =>
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ timezone: 'America/Mexico_City' }),
    ),
  )
})

test('a saved zone that is not one of the usual ones is kept', async () => {
  show({ timezone: 'Europe/Madrid' })

  const picker = await screen.findByLabelText('Spell the date and time in')
  expect((picker as HTMLSelectElement).value).toBe('Europe/Madrid')
})

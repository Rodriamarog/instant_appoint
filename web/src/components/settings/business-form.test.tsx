import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { BusinessForm } from '@/components/settings/business-form'
import { DEFAULT_SETTINGS, type BusinessSettings } from '@/lib/business-settings'

const onSaveName = vi.fn(() => Promise.resolve())
const onSaveSettings = vi.fn(() => Promise.resolve())

function show(settings: Partial<BusinessSettings> = {}, businessName = 'Clínica Norte') {
  render(
    <BusinessForm
      businessName={businessName}
      settings={{ ...DEFAULT_SETTINGS, ...settings }}
      onSaveName={onSaveName}
      onSaveSettings={onSaveSettings}
    />,
  )
}

beforeEach(() => {
  onSaveName.mockClear()
  onSaveSettings.mockClear()
})

afterEach(cleanup)

test('the zone the business runs in can be chosen', async () => {
  const user = userEvent.setup()
  show()

  await user.selectOptions(screen.getByLabelText('Zona horaria'), 'America/Mexico City')
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() =>
    expect(onSaveSettings).toHaveBeenCalledWith({ timezone: 'America/Mexico_City' }),
  )
  expect(onSaveName).not.toHaveBeenCalled()
})

test('a saved zone that is not one of the usual ones is kept', () => {
  show({ timezone: 'Europe/Madrid' })

  expect((screen.getByLabelText('Zona horaria') as HTMLSelectElement).value).toBe('Europe/Madrid')
})

test('the business can be renamed, and only the name is written', async () => {
  const user = userEvent.setup()
  show()

  const name = screen.getByLabelText('Nombre del negocio')
  await user.clear(name)
  await user.type(name, 'Clínica Sur ')
  await user.click(screen.getByRole('button', { name: 'Guardar' }))

  await waitFor(() => expect(onSaveName).toHaveBeenCalledWith('Clínica Sur'))
  expect(onSaveSettings).not.toHaveBeenCalled()
})

test('nothing to save until something changes', () => {
  show()

  expect((screen.getByRole('button', { name: 'Guardar' }) as HTMLButtonElement).disabled).toBe(true)
})

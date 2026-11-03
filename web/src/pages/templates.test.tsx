import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { MemoryRouter } from 'react-router'

import { pb } from '@/lib/pocketbase'
import { TemplatesPage } from '@/pages/templates'

/** One approved template with two placeholders, and one still pending. */
const metaTemplates = [
  {
    name: 'appointment_reminder',
    status: 'APPROVED',
    category: 'UTILITY',
    language: 'es',
    components: [
      { type: 'BODY', text: 'Hola {{1}}, te recordamos tu cita {{2}}.' },
    ],
  },
  {
    name: 'welcome_message',
    status: 'PENDING',
    category: 'MARKETING',
    language: 'en_US',
    components: [{ type: 'BODY', text: 'Welcome aboard!' }],
  },
]

type StubbedCall = { url: string; init?: RequestInit }

/**
 * Stands in for the three template endpoints and records what the page asked
 * for, so a test can assert the request rather than only the rendering.
 */
function stubServer(
  replies: Record<string, { ok?: boolean; body: unknown }> = {},
) {
  const calls: StubbedCall[] = []

  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, init })

      const reply = replies[url] ?? {
        body: { templates: metaTemplates },
      }

      return Promise.resolve({
        ok: reply.ok ?? true,
        json: () => Promise.resolve(reply.body),
      } as Response)
    }),
  )

  return calls
}

/** Gives the signed-in user a connected number, or none. */
function stubAccounts(records: unknown[]) {
  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList: () => Promise.resolve(records),
  } as unknown as ReturnType<typeof pb.collection>)
}

const connected = [{ id: 'acc1', waba_id: '100000000000001' }]

function renderPage() {
  return render(
    <MemoryRouter>
      <TemplatesPage />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  pb.authStore.save('the-session-token', {
    id: 'user123',
    collectionId: '_pb_users_auth_',
    collectionName: 'users',
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('lists the templates with their approval status', async () => {
  stubAccounts(connected)
  const calls = stubServer()

  renderPage()

  expect(await screen.findByText('appointment_reminder')).toBeDefined()
  expect(screen.getByText('APPROVED')).toBeDefined()
  expect(screen.getByText('welcome_message')).toBeDefined()
  expect(screen.getByText('PENDING')).toBeDefined()
  expect(
    screen.getByText('Hola {{1}}, te recordamos tu cita {{2}}.'),
  ).toBeDefined()

  // The endpoint holds the account token, so the session has to identify the
  // caller; the account itself is never named in the request.
  const list = calls.find((call) => call.url === '/api/whatsapp/templates')
  expect(list).toBeDefined()
  expect(
    (list?.init?.headers as Record<string, string>)?.Authorization,
  ).toBe('Bearer the-session-token')
})

test('asks for a connected number before asking Meta for anything', async () => {
  stubAccounts([])
  const calls = stubServer()

  renderPage()

  expect(await screen.findByText(/conecta un número de whatsapp/i)).toBeDefined()
  expect(calls).toHaveLength(0)
})

test("shows the server's message when Meta refuses the list", async () => {
  stubAccounts(connected)
  stubServer({
    '/api/whatsapp/templates': {
      ok: false,
      body: {
        message: 'Meta rejected the request: Session has expired.',
        status: 500,
        data: {},
      },
    },
  })

  renderPage()

  expect(await screen.findByText(/session has expired/i)).toBeDefined()
})

test('will not submit a draft that is missing a name or a body', async () => {
  stubAccounts(connected)
  const calls = stubServer()
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')
  await user.click(screen.getByRole('button', { name: /nueva plantilla/i }))
  await user.click(screen.getByRole('button', { name: /enviar a aprobación/i }))

  expect(await screen.findByText(/el nombre es obligatorio/i)).toBeDefined()
  expect(screen.getByText(/el mensaje es obligatorio/i)).toBeDefined()
  expect(
    calls.filter((call) => call.url.endsWith('/create')),
  ).toHaveLength(0)
})

test('will not submit a name Meta would reject', async () => {
  stubAccounts(connected)
  const calls = stubServer()
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')
  await user.click(screen.getByRole('button', { name: /nueva plantilla/i }))
  await user.type(screen.getByLabelText(/nombre/i), 'Appointment Reminder!')
  await user.type(screen.getByLabelText(/mensaje/i), 'Hola')
  await user.click(screen.getByRole('button', { name: /enviar a aprobación/i }))

  expect(await screen.findByText(/solo minúsculas, números/i)).toBeDefined()
  expect(calls.filter((call) => call.url.endsWith('/create'))).toHaveLength(0)
})

test('submits a complete draft and reports that it is pending', async () => {
  stubAccounts(connected)
  const calls = stubServer({
    '/api/whatsapp/templates/create': {
      body: { success: true, template: { id: '1', status: 'PENDING' } },
    },
  })
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')
  await user.click(screen.getByRole('button', { name: /nueva plantilla/i }))
  await user.type(screen.getByLabelText(/nombre/i), 'visit_summary')
  await user.selectOptions(screen.getByLabelText(/categoría/i), 'MARKETING')
  await user.selectOptions(screen.getByLabelText(/idioma/i), 'en_US')
  // user-event reads "{{" as an escaped "{", so a literal {{1}} is doubled.
  await user.type(
    screen.getByLabelText(/mensaje/i),
    'Thanks for visiting {{{{1}}.',
  )
  await user.click(screen.getByRole('button', { name: /enviar a aprobación/i }))

  // "PENDING" alone is also a badge in the list, so the assertion names the
  // sentence the form shows rather than the word.
  expect(await screen.findByText(/se envió a aprobación/i)).toBeDefined()

  const create = calls.find((call) => call.url.endsWith('/create'))
  expect(create).toBeDefined()
  expect(JSON.parse(String(create?.init?.body))).toEqual({
    name: 'visit_summary',
    category: 'MARKETING',
    language: 'en_US',
    body: 'Thanks for visiting {{1}}.',
    buttons: [],
  })
})

// A reminder the patient answers with one tap. The buttons are part of the
// template Meta approves, so they are chosen here or not at all.
test('submits a template carrying Confirm and Cancel quick replies', async () => {
  stubAccounts(connected)
  const calls = stubServer({
    '/api/whatsapp/templates/create': {
      body: { success: true, template: { id: '1', status: 'PENDING' } },
    },
  })
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')
  await user.click(screen.getByRole('button', { name: /nueva plantilla/i }))
  await user.type(screen.getByLabelText(/^nombre$/i), 'cita_recordatorio')
  await user.type(screen.getByLabelText(/mensaje/i), 'Hola {{{{1}}.')
  await user.click(screen.getByRole('checkbox', { name: /botones/i }))
  await user.click(screen.getByRole('button', { name: /enviar a aprobación/i }))

  await screen.findByText(/se envió a aprobación/i)

  const create = calls.find((call) => call.url.endsWith('/create'))
  expect(JSON.parse(String(create?.init?.body))).toEqual({
    name: 'cita_recordatorio',
    category: 'UTILITY',
    language: 'es',
    body: 'Hola {{1}}.',
    buttons: ['Confirmar', 'Cancelar'],
  })
})

// The same template exists once per language at Meta, and the buttons are
// approved inside it: an English template with a Spanish "Confirmar" on it
// cannot be relabelled per recipient later.
test('relabels the buttons with the language, but not wording of your own', async () => {
  stubAccounts(connected)
  const calls = stubServer({
    '/api/whatsapp/templates/create': {
      body: { success: true, template: { id: '1', status: 'PENDING' } },
    },
  })
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')
  await user.click(screen.getByRole('button', { name: /nueva plantilla/i }))
  await user.type(screen.getByLabelText(/^nombre$/i), 'appointment_reminder')
  await user.type(screen.getByLabelText(/mensaje/i), 'Hi {{{{1}}.')
  await user.click(screen.getByRole('checkbox', { name: /botones/i }))
  await user.selectOptions(screen.getByLabelText(/idioma/i), 'en_US')

  expect((screen.getByLabelText(/botón 1/i) as HTMLInputElement).value).toBe(
    'Confirm',
  )

  // Wording the person typed is theirs. Switching language after it must not
  // throw it away.
  await user.clear(screen.getByLabelText(/botón 2/i))
  await user.type(screen.getByLabelText(/botón 2/i), 'Not this time')
  await user.selectOptions(screen.getByLabelText(/idioma/i), 'es')
  await user.click(screen.getByRole('button', { name: /enviar a aprobación/i }))

  await screen.findByText(/se envió a aprobación/i)

  const create = calls.find((call) => call.url.endsWith('/create'))
  expect(JSON.parse(String(create?.init?.body))).toMatchObject({
    language: 'es',
    buttons: ['Confirm', 'Not this time'],
  })
})

// Whether the submitted template has its buttons is only knowable from what
// Meta sends back, and it is knowable too late to change: approval is what
// freezes them.
test('shows the buttons a template carries next to its review status', async () => {
  stubAccounts(connected)
  stubServer({
    '/api/whatsapp/templates': {
      body: {
        templates: [
          {
            name: 'cita_recordatorio',
            status: 'PENDING',
            category: 'UTILITY',
            language: 'es',
            components: [
              { type: 'BODY', text: 'Hola {{1}}.' },
              {
                type: 'BUTTONS',
                buttons: [
                  { type: 'QUICK_REPLY', text: 'Confirmar' },
                  { type: 'QUICK_REPLY', text: 'Cancelar' },
                ],
              },
            ],
          },
        ],
      },
    },
  })

  renderPage()

  expect(await screen.findByText('cita_recordatorio')).toBeDefined()
  expect(screen.getByText('PENDING')).toBeDefined()
  expect(screen.getByText('Confirmar')).toBeDefined()
  expect(screen.getByText('Cancelar')).toBeDefined()
})

// Every template that existed before buttons did has none, and the reminder
// a business already relies on is one of them.
test('a template with no buttons still reads and sends as it always did', async () => {
  stubAccounts(connected)
  const calls = stubServer({
    '/api/whatsapp/templates/send': {
      body: { success: true, message_id: 'wamid.TEST' },
    },
  })
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')

  expect(screen.queryByText('Confirmar')).toBeNull()
  expect(
    screen.getByText('Hola {{1}}, te recordamos tu cita {{2}}.'),
  ).toBeDefined()

  await user.click(screen.getByRole('button', { name: /enviar prueba/i }))
  await user.type(screen.getByLabelText(/enviar a/i), '5216641234567')
  await user.type(screen.getByLabelText('{{1}}'), 'Ana')
  await user.type(screen.getByLabelText('{{2}}'), 'mañana')
  await user.click(screen.getByRole('button', { name: /^enviar$/i }))

  await waitFor(() => {
    expect(calls.some((call) => call.url.endsWith('/send'))).toBe(true)
  })
  const sent = calls.find((call) => call.url.endsWith('/send'))
  expect(JSON.parse(String(sent?.init?.body))).toEqual({
    template_name: 'appointment_reminder',
    language_code: 'es',
    to: '5216641234567',
    body_parameters: ['Ana', 'mañana'],
  })
})

test("shows Meta's rejection when a draft is refused", async () => {
  stubAccounts(connected)
  stubServer({
    '/api/whatsapp/templates/create': {
      ok: false,
      body: {
        message:
          'Meta rejected the request: Template name (visit_summary) already exists in es',
        status: 500,
        data: {},
      },
    },
  })
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')
  await user.click(screen.getByRole('button', { name: /nueva plantilla/i }))
  await user.type(screen.getByLabelText(/nombre/i), 'visit_summary')
  await user.type(screen.getByLabelText(/mensaje/i), 'Thanks!')
  await user.click(screen.getByRole('button', { name: /enviar a aprobación/i }))

  expect(await screen.findByText(/already exists/i)).toBeDefined()
})

test('sends a template with a value for each of its placeholders', async () => {
  stubAccounts(connected)
  const calls = stubServer({
    '/api/whatsapp/templates/send': {
      body: { success: true, message_id: 'wamid.TEST' },
    },
  })
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')
  await user.click(screen.getByRole('button', { name: /enviar prueba/i }))

  // The number is the tester's to supply: the page ships with nobody's.
  const phone = screen.getByLabelText(/enviar a/i) as HTMLInputElement
  expect(phone.value).toBe('')

  await user.type(phone, '5216641234567')
  await user.type(screen.getByLabelText(/\{\{1\}\}/), 'Ana')
  await user.type(screen.getByLabelText(/\{\{2\}\}/), 'mañana a las 10')
  await user.click(screen.getByRole('button', { name: /^enviar$/i }))

  await waitFor(() => {
    expect(calls.some((call) => call.url.endsWith('/send'))).toBe(true)
  })

  const sent = calls.find((call) => call.url.endsWith('/send'))
  expect(JSON.parse(String(sent?.init?.body))).toEqual({
    template_name: 'appointment_reminder',
    language_code: 'es',
    to: '5216641234567',
    body_parameters: ['Ana', 'mañana a las 10'],
  })
  expect(await screen.findByText(/mensaje enviado/i)).toBeDefined()
})

test('will not send without a number or without every placeholder filled', async () => {
  stubAccounts(connected)
  const calls = stubServer()
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')
  await user.click(screen.getByRole('button', { name: /enviar prueba/i }))
  await user.click(screen.getByRole('button', { name: /^enviar$/i }))

  expect(await screen.findByText(/escribe el número al que enviar/i)).toBeDefined()
  expect(calls.filter((call) => call.url.endsWith('/send'))).toHaveLength(0)

  await user.type(screen.getByLabelText(/enviar a/i), '5216641234567')
  await user.type(screen.getByLabelText(/\{\{1\}\}/), 'Ana')
  await user.click(screen.getByRole('button', { name: /^enviar$/i }))

  expect(await screen.findByText(/rellena todos los valores/i)).toBeDefined()
  expect(calls.filter((call) => call.url.endsWith('/send'))).toHaveLength(0)
})

test('offers no test send for a template Meta has not approved', async () => {
  stubAccounts(connected)
  stubServer()
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('welcome_message')

  // Exactly one of the two templates is approved, so exactly one send button.
  expect(screen.getAllByRole('button', { name: /enviar prueba/i })).toHaveLength(1)
  await user.click(screen.getByRole('button', { name: /enviar prueba/i }))
  expect(screen.getByLabelText(/enviar a/i)).toBeDefined()
})

test("shows the server's message when a send is refused", async () => {
  stubAccounts(connected)
  stubServer({
    '/api/whatsapp/templates/send': {
      ok: false,
      body: {
        message:
          'Meta rejected the request: Recipient phone number not in allowed list',
        status: 500,
        data: {},
      },
    },
  })
  const user = userEvent.setup()

  renderPage()
  await screen.findByText('appointment_reminder')
  await user.click(screen.getByRole('button', { name: /enviar prueba/i }))
  await user.type(screen.getByLabelText(/enviar a/i), '15550000000')
  await user.type(screen.getByLabelText(/\{\{1\}\}/), 'Ana')
  await user.type(screen.getByLabelText(/\{\{2\}\}/), 'hoy')
  await user.click(screen.getByRole('button', { name: /^enviar$/i }))

  expect(await screen.findByText(/not in allowed list/i)).toBeDefined()
})

import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import {
  createAppointment,
  languageForCountryCode,
  setAppointmentStatus,
  updateAppointment,
} from '@/lib/appointments'
import { pb } from '@/lib/pocketbase'

/**
 * The seam is the PocketBase SDK. What is under test is what this module
 * writes into the collection — every appointment the SPA creates is one a
 * person at the business booked, and every status it sets is one a person at
 * the business set, so the provenance is this module's to stamp rather than
 * each caller's to remember.
 */
const create = vi.fn()
const update = vi.fn()

const signedInUser = {
  id: 'user123',
  collectionId: '_pb_users_auth_',
  collectionName: 'users',
}

/** Fixed, so "when the status was set" is an assertion and not a race. */
const NOW = new Date('2026-11-02T12:00:00.000Z')

const draft = {
  client_name: 'Ana Ruiz',
  client_phone: '5215512345678',
  notes: 'Alérgica al látex.',
  start_time: '2026-11-09T17:00:00.000Z',
  end_time: '2026-11-09T18:00:00.000Z',
  location: 'Consultorio 2',
  language: 'es',
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(NOW)

  create.mockReset().mockResolvedValue({ id: 'evt123' })
  update.mockReset().mockResolvedValue({ id: 'evt123' })

  vi.spyOn(pb, 'collection').mockReturnValue({
    create,
    update,
  } as unknown as ReturnType<typeof pb.collection>)

  pb.authStore.save('not-a-real-token', signedInUser)
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('an appointment booked here starts pending, not confirmed', async () => {
  // Nobody has asked the patient anything yet. A new booking that claimed to
  // be confirmed would put a clinic's whole day on a screen as settled.
  await createAppointment(draft)

  expect(create.mock.calls[0][0]).toMatchObject({ status: 'pending' })
})

test('an appointment booked here is recorded as the business’s own doing', async () => {
  // The SPA is the staff-facing app and has no other kind of user, so both
  // halves of the provenance are known here rather than asked for.
  await createAppointment(draft)

  expect(create.mock.calls[0][0]).toMatchObject({
    created_by: 'staff',
    status_set_by: 'staff',
    status_set_at: '2026-11-02T12:00:00.000Z',
  })
})

test('setting a status records who set it and when', async () => {
  await setAppointmentStatus('evt123', 'confirmed')

  expect(update.mock.calls[0][0]).toBe('evt123')
  expect(update.mock.calls[0][1]).toEqual({
    status: 'confirmed',
    status_set_by: 'staff',
    status_set_at: '2026-11-02T12:00:00.000Z',
  })
})

test('the moment a status was set is written in the stored timestamp shape', async () => {
  // Text column, compared byte by byte like every other timestamp here.
  await setAppointmentStatus('evt123', 'cancelled')

  expect(update.mock.calls[0][1].status_set_at).toMatch(
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/,
  )
})

test('editing the details of an appointment does not relabel who last set its status', async () => {
  // Correcting a phone number is not the patient confirming, and it is not
  // staff re-confirming either. A save that restamped the provenance would
  // make the one question the screen answers unanswerable.
  await updateAppointment('evt123', draft)

  const body = update.mock.calls[0][1]
  expect(body.status).toBeUndefined()
  expect(body.status_set_by).toBeUndefined()
  expect(body.status_set_at).toBeUndefined()
  expect(body).toMatchObject({ client_name: 'Ana Ruiz' })
})

test('the reminder language is guessed from the dialling prefix', () => {
  // Mexican numbers read Spanish and US numbers read English, which is the
  // right guess most of the time on a border clinic's list. It is only a
  // guess: a Mexican patient who prefers English is unremarkable here, which
  // is why the dialog lets staff change it.
  expect(languageForCountryCode('521')).toBe('es')
  expect(languageForCountryCode('1')).toBe('en_US')
})

test('a prefix nobody has mapped is reminded in Spanish', () => {
  // The clinic is in Tijuana and the template every one of them starts with
  // is the Spanish one. Guessing English for an unknown prefix would write
  // to a patient in a language the business may not even have approved.
  expect(languageForCountryCode('49')).toBe('es')
})

test('an appointment is written with the language its reminder goes out in', async () => {
  // The reminder job reads this column and nothing else: a create that
  // dropped it would leave the appointment on the business's own default,
  // which is the behaviour this replaced.
  await createAppointment({ ...draft, language: 'en_US' })

  expect(create.mock.calls[0][0]).toMatchObject({ language: 'en_US' })
})

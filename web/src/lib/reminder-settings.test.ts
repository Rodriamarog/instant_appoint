import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { pb } from '@/lib/pocketbase'
import {
  DEFAULT_SETTINGS,
  loadReminderSettings,
  saveReminderSettings,
} from '@/lib/reminder-settings'

/**
 * The seam is the PocketBase SDK. What this module is for is the three
 * decisions above it: what to show when nobody has saved anything, whether a
 * save is a create or an edit, and that a create carries its owner — a create
 * without one is refused by the collection's rule, which runs before any
 * hook, so the stamping has to happen here.
 */
const getFirstListItem = vi.fn()
const create = vi.fn()
const update = vi.fn()

const signedInUser = {
  id: 'user123',
  collectionId: '_pb_users_auth_',
  collectionName: 'users',
}

/** A stored row, as PocketBase returns it. */
const stored = {
  id: 'set123',
  user_id: signedInUser.id,
  timing_minutes: 120,
  is_active: false,
  template_name: 'appointment_reminder',
  template_language: 'es',
  timezone: 'America/Tijuana',
}

/** The error the SDK raises when getFirstListItem matches nothing. */
function notFound() {
  return Object.assign(new Error('The requested resource wasn’t found.'), {
    status: 404,
  })
}

beforeEach(() => {
  getFirstListItem.mockReset()
  create.mockReset().mockResolvedValue(stored)
  update.mockReset().mockResolvedValue(stored)

  vi.spyOn(pb, 'collection').mockReturnValue({
    getFirstListItem,
    create,
    update,
  } as unknown as ReturnType<typeof pb.collection>)

  pb.authStore.save('not-a-real-token', signedInUser)
})

afterEach(() => {
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('somebody who has never opened the dialog gets a working default', async () => {
  getFirstListItem.mockRejectedValue(notFound())

  await expect(loadReminderSettings()).resolves.toEqual(DEFAULT_SETTINGS)
})

test('the default sends a reminder a day ahead, in Spanish, Tijuana time', async () => {
  // Pinned rather than read back from the module: these are the values #9
  // formats a reminder with, and a silent change to any of them changes when
  // and in what language a real message arrives.
  expect(DEFAULT_SETTINGS.timing_minutes).toBe(1440)
  expect(DEFAULT_SETTINGS.is_active).toBe(true)
  expect(DEFAULT_SETTINGS.template_language).toBe('es')
  expect(DEFAULT_SETTINGS.timezone).toBe('America/Tijuana')
})

test('saved settings come back instead of the default', async () => {
  getFirstListItem.mockResolvedValue(stored)

  await expect(loadReminderSettings()).resolves.toEqual(stored)
})

test('a failure that is not "nothing saved yet" is not mistaken for one', async () => {
  // Returning the default here would quietly show somebody else's settings
  // back to them as their own, and a save would then overwrite the real row.
  getFirstListItem.mockRejectedValue(
    Object.assign(new Error('Something went wrong'), { status: 500 }),
  )

  await expect(loadReminderSettings()).rejects.toThrow()
})

test('the first save creates a row stamped with its owner', async () => {
  getFirstListItem.mockRejectedValue(notFound())

  await saveReminderSettings({ ...DEFAULT_SETTINGS, timing_minutes: 30 })

  expect(update).not.toHaveBeenCalled()
  expect(create).toHaveBeenCalledTimes(1)
  expect(create.mock.calls[0][0]).toMatchObject({
    user_id: signedInUser.id,
    timing_minutes: 30,
  })
})

test('a later save edits the row that is already there', async () => {
  getFirstListItem.mockResolvedValue(stored)

  await saveReminderSettings({ ...stored, timing_minutes: 720 })

  expect(create).not.toHaveBeenCalled()
  expect(update).toHaveBeenCalledWith('set123', expect.objectContaining({ timing_minutes: 720 }))
})

test('an edit never tries to move the row to another owner', async () => {
  // The update rule refuses it anyway; sending it would turn every save into
  // a 400 for no gain.
  getFirstListItem.mockResolvedValue(stored)

  await saveReminderSettings({ ...stored, is_active: false })

  expect(update.mock.calls[0][1]).not.toHaveProperty('user_id')
})

test('saving without a session fails rather than writing an ownerless row', async () => {
  pb.authStore.clear()
  getFirstListItem.mockRejectedValue(notFound())

  await expect(saveReminderSettings(DEFAULT_SETTINGS)).rejects.toThrow()
  expect(create).not.toHaveBeenCalled()
})

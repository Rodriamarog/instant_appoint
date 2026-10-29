import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { pb } from '@/lib/pocketbase'
import {
  COLLECTION,
  displayNumber,
  listConnectedAccounts,
} from '@/lib/whatsapp-accounts'

type ListOptions = { filter?: string; sort?: string }

/** Records the collection and query the module asked PocketBase for. */
function spyOnCollection(records: unknown[] = []) {
  const asked: { collection: string; options: ListOptions }[] = []

  vi.spyOn(pb, 'collection').mockImplementation(
    (collection: string) =>
      ({
        getFullList: (options: ListOptions) => {
          asked.push({ collection, options: options ?? {} })
          return Promise.resolve(records)
        },
      }) as unknown as ReturnType<typeof pb.collection>,
  )

  return asked
}

beforeEach(() => {
  pb.authStore.save('token', {
    id: 'user123',
    collectionId: '_pb_users_auth_',
    collectionName: 'users',
  })
})

afterEach(() => {
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('it asks the whatsapp_accounts collection for the connected business numbers', async () => {
  const asked = spyOnCollection()

  await listConnectedAccounts()

  expect(asked).toHaveLength(1)
  expect(asked[0].collection).toBe(COLLECTION)
  expect(asked[0].options.filter).toContain('account_type = "business_api"')
  expect(asked[0].options.filter).toContain('is_active = true')
  // A retired QR account is not a Cloud API connection and must not be listed
  // as one.
  expect(asked[0].options.filter).toContain('user123')
})

test('the query never mentions a hidden field', async () => {
  // access_token, session_id and qr_code are hidden. PocketBase answers a
  // client filter or sort naming one with a hard 400, so a query that touched
  // one would break the page outright rather than degrade.
  const asked = spyOnCollection()

  await listConnectedAccounts()

  const { filter = '', sort = '' } = asked[0].options
  for (const hidden of ['access_token', 'session_id', 'qr_code']) {
    expect(filter).not.toContain(hidden)
    expect(sort).not.toContain(hidden)
  }
})

test('an expired session asks for nobody’s accounts rather than everybody’s', async () => {
  pb.authStore.clear()
  const asked = spyOnCollection()

  await listConnectedAccounts()

  expect(asked[0].options.filter).toContain("user_id = ''")
})

test('the displayed number falls back to the id Meta assigned', () => {
  const account = {
    id: 'a',
    phone_number: '',
    phone_number_id: '200000000000002',
    waba_id: 'w',
    verified_name: 'Dev Clinic',
  }

  // A real Cloud API account has no phone_number: only the retired QR service
  // ever wrote one.
  expect(displayNumber(account)).toBe('+200000000000002')
  expect(displayNumber({ ...account, phone_number: '5216641234567' })).toBe(
    '+5216641234567',
  )
})

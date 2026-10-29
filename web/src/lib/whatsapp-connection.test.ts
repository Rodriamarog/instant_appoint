import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { pb } from '@/lib/pocketbase'
import { isWhatsAppConnected } from '@/lib/whatsapp-connection'

const signedInUser = {
  id: 'user123',
  collectionId: '_pb_users_auth_',
  collectionName: 'users',
}

const getFirstListItem = vi.fn()

beforeEach(() => {
  getFirstListItem.mockReset()

  vi.spyOn(pb, 'collection').mockReturnValue({
    getFirstListItem,
  } as unknown as ReturnType<typeof pb.collection>)

  pb.authStore.save('not-a-real-token', signedInUser)
})

afterEach(() => {
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('a live Cloud API number counts as connected', async () => {
  getFirstListItem.mockResolvedValue({ id: 'acc1', phone_number_id: '200000000000002' })

  await expect(isWhatsAppConnected()).resolves.toBe(true)
})

test('nobody connected means not connected, which is not an error', async () => {
  getFirstListItem.mockRejectedValue(
    Object.assign(new Error('The requested resource wasn’t found.'), { status: 404 }),
  )

  await expect(isWhatsAppConnected()).resolves.toBe(false)
})

test('only a live business account counts', async () => {
  // The retired QR path wrote account_type "web" rows and left disconnected
  // ones behind with is_active false. Neither can deliver a reminder.
  getFirstListItem.mockResolvedValue({ id: 'acc1' })

  await isWhatsAppConnected()

  const filter = getFirstListItem.mock.calls[0][0] as string
  expect(filter).toContain('account_type = "business_api"')
  expect(filter).toContain('is_active = true')
  expect(filter).toContain(signedInUser.id)
})

test('the query never mentions the access token', async () => {
  // access_token is a hidden field; PocketBase answers a client filter that
  // names one with a hard 400, which would read as "not connected".
  getFirstListItem.mockResolvedValue({ id: 'acc1' })

  await isWhatsAppConnected()

  expect(getFirstListItem.mock.calls[0][0]).not.toContain('access_token')
})

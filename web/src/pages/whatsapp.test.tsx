import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { pb } from '@/lib/pocketbase'
import { WhatsAppPage } from '@/pages/whatsapp'

type ListOptions = { filter?: string; sort?: string }

const connectedAccount = {
  id: 'acc1',
  phone_number: '5216641234567',
  phone_number_id: '200000000000002',
  waba_id: '100000000000001',
  verified_name: 'Dev Clinic',
  status: 'connected',
}

/**
 * Stands in for the collection the page lists, and records the query it was
 * given so a test can check what the page asked for.
 */
function stubAccounts(records: unknown[]) {
  const calls: ListOptions[] = []

  vi.spyOn(pb, 'collection').mockReturnValue({
    getFullList: (options: ListOptions) => {
      calls.push(options ?? {})
      return Promise.resolve(records)
    },
  } as unknown as ReturnType<typeof pb.collection>)

  return calls
}

beforeEach(() => {
  pb.authStore.save('token', {
    id: 'user123',
    collectionId: '_pb_users_auth_',
    collectionName: 'users',
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('a connected number is listed with its name, number and WABA', async () => {
  stubAccounts([connectedAccount])

  render(<WhatsAppPage />)

  expect(await screen.findByText('Dev Clinic')).toBeDefined()
  expect(screen.getByText('+5216641234567')).toBeDefined()
  expect(screen.getByText(/100000000000001/)).toBeDefined()
  expect(screen.getByText('Connected')).toBeDefined()
})

test("falls back to the phone number id when Meta's number was never stored", async () => {
  // The Cloud API path writes phone_number_id but never phone_number, so
  // this is what a real connected account looks like.
  stubAccounts([{ ...connectedAccount, phone_number: '' }])

  render(<WhatsAppPage />)

  expect(await screen.findByText('+200000000000002')).toBeDefined()
})

test('says so when no number is connected', async () => {
  stubAccounts([])

  render(<WhatsAppPage />)

  expect(await screen.findByText('Not connected')).toBeDefined()
})

test('the query never mentions the access token', async () => {
  // access_token is a hidden field. PocketBase answers a client filter or
  // sort that names one with a hard 400, so a query that touched it would
  // break the page outright rather than degrade.
  const calls = stubAccounts([connectedAccount])

  render(<WhatsAppPage />)
  await screen.findByText('Dev Clinic')

  expect(calls.length).toBeGreaterThan(0)
  for (const { filter, sort } of calls) {
    expect(filter ?? '').not.toContain('access_token')
    expect(sort ?? '').not.toContain('access_token')
  }
})

test('visiting the page opens no connection of its own', async () => {
  // The page this replaces opened a socket.io connection to a local service
  // on every mount, whether or not that service was running.
  const fetchSpy = vi.spyOn(globalThis, 'fetch')
  stubAccounts([connectedAccount])

  render(<WhatsAppPage />)
  await screen.findByText('Dev Clinic')

  expect(fetchSpy).not.toHaveBeenCalled()
})

import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { register, signIn } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'
import { pb } from '@/lib/pocketbase'

/**
 * These two were written inline in the login and register pages, which made
 * the verification rule a property of a form rather than of the session.
 */

const authWithPassword = vi.fn()
const create = vi.fn()
const requestVerification = vi.fn()

/** Signs the given account in, the way PocketBase's SDK would. */
function sessionFor(verified: boolean) {
  return () => {
    pb.authStore.save('the-session-token', {
      id: 'user123',
      collectionId: '_pb_users_auth_',
      collectionName: 'users',
      verified,
    })

    return Promise.resolve({ token: 'the-session-token' })
  }
}

beforeEach(() => {
  authWithPassword.mockReset()
  create.mockReset().mockResolvedValue({ id: 'user123' })
  requestVerification.mockReset().mockResolvedValue(true)

  vi.spyOn(pb, 'collection').mockReturnValue({
    authWithPassword,
    create,
    requestVerification,
  } as unknown as ReturnType<typeof pb.collection>)
})

afterEach(() => {
  vi.restoreAllMocks()
  pb.authStore.clear()
})

test('a verified account is signed in and stays signed in', async () => {
  authWithPassword.mockImplementation(sessionFor(true))

  await signIn('ana@instantappoint.test', 'devpassword123')

  // isValid would mean parsing a real JWT; what matters here is that
  // the session was not torn down again.
  expect(pb.authStore.record?.id).toBe('user123')
})

/**
 * PocketBase does not enforce this: authWithPassword succeeds for an
 * unverified account. Mail delivery is not configured yet, so every account
 * created through the form is unverified — letting them in would make the
 * pending verification work a no-op.
 */
test('an unverified account is refused, and the session it opened is cleared again', async () => {
  authWithPassword.mockImplementation(sessionFor(false))

  await expect(signIn('ana@instantappoint.test', 'devpassword123')).rejects.toThrow(
    /verify your email/i,
  )
  expect(pb.authStore.record).toBeNull()
})

test('registering asks for the verification email and says it went out', async () => {
  const result = await register({
    email: 'ana@instantappoint.test',
    password: 'devpassword123',
    name: 'Ana Ruiz',
    businessName: 'Ruiz Dental',
  })

  expect(result.verificationSent).toBe(true)
  expect(create).toHaveBeenCalledWith(
    expect.objectContaining({
      email: 'ana@instantappoint.test',
      passwordConfirm: 'devpassword123',
      business_name: 'Ruiz Dental',
      emailVisibility: true,
    }),
  )
})

/**
 * Without configured mail delivery the verification request fails, and the
 * account exists either way. A failure here must not read as a failed signup.
 */
test('an undeliverable verification email does not fail the signup', async () => {
  requestVerification.mockRejectedValue(new Error('no mail server configured'))

  const result = await register({
    email: 'ana@instantappoint.test',
    password: 'devpassword123',
    name: 'Ana Ruiz',
    businessName: '',
  })

  expect(result.verificationSent).toBe(false)
})

test('an account that could not be created is a failure, not a quiet success', async () => {
  create.mockRejectedValue(new Error('The email is invalid or already in use.'))

  await expect(
    register({ email: 'taken@instantappoint.test', password: 'x', name: '', businessName: '' }),
  ).rejects.toThrow('The email is invalid or already in use.')
  expect(requestVerification).not.toHaveBeenCalled()
})

test('errorMessage prefers what was thrown and falls back when there is nothing to say', () => {
  expect(errorMessage(new Error('Meta rejected the request'), 'fallback')).toBe(
    'Meta rejected the request',
  )
  expect(errorMessage(new Error(''), 'fallback')).toBe('fallback')
  expect(errorMessage('a bare string', 'fallback')).toBe('fallback')
  expect(errorMessage(undefined, 'fallback')).toBe('fallback')
})

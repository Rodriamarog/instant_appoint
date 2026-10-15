import { describe, expect, it } from 'vitest'
import { parseSignupMessage, resolvePhoneNumberId } from '@/lib/embedded-signup'

const msg = (event: string, data: unknown = {}) => JSON.stringify({ type: 'WA_EMBEDDED_SIGNUP', event, data })

describe('parseSignupMessage', () => {
  it('reads the ids from a normal FINISH', () => {
    expect(parseSignupMessage(msg('FINISH', { waba_id: '111', phone_number_id: '222' }))).toEqual({
      kind: 'finish',
      wabaId: '111',
      phoneNumberId: '222',
    })
  })

  it('treats the WhatsApp Business app (coexistence) finish as a finish', () => {
    expect(
      parseSignupMessage(msg('FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING', { waba_id: '111', phone_number_id: '222' }))
    ).toEqual({ kind: 'finish', wabaId: '111', phoneNumberId: '222' })
  })

  it('leaves the phone number id out when Meta does not send it', () => {
    expect(parseSignupMessage(msg('FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING', { waba_id: '111' }))).toEqual({
      kind: 'finish',
      wabaId: '111',
      phoneNumberId: undefined,
    })
  })

  it('ignores a finish without a WABA id', () => {
    expect(parseSignupMessage(msg('FINISH', {}))).toBeNull()
  })

  it('reports cancel and error', () => {
    expect(parseSignupMessage(msg('CANCEL', { current_step: 'x' }))).toEqual({ kind: 'cancel' })
    expect(parseSignupMessage(msg('ERROR', { error_message: 'boom' }))).toEqual({ kind: 'error', message: 'boom' })
    expect(parseSignupMessage(msg('ERROR'))).toEqual({ kind: 'error', message: 'Unknown error from Facebook' })
  })

  it('ignores other messages', () => {
    expect(parseSignupMessage('not json')).toBeNull()
    expect(parseSignupMessage(JSON.stringify({ type: 'OTHER', event: 'FINISH', data: { waba_id: '1' } }))).toBeNull()
    expect(parseSignupMessage({ some: 'object' })).toBeNull()
  })
})

function fakeFetch(body: unknown, ok = true) {
  const calls: string[] = []
  const fetchFn = async (url: string | URL | Request) => {
    calls.push(String(url))
    return new Response(JSON.stringify(body), { status: ok ? 200 : 400 })
  }
  return { fetchFn: fetchFn as typeof fetch, calls }
}

describe('resolvePhoneNumberId', () => {
  it('keeps the id from the signup when there is one, without calling Meta', async () => {
    const { fetchFn, calls } = fakeFetch({})
    expect(await resolvePhoneNumberId('111', '222', 'tok', fetchFn)).toBe('222')
    expect(calls).toEqual([])
  })

  it("looks up the WABA's only number when the signup did not send one", async () => {
    const { fetchFn, calls } = fakeFetch({ data: [{ id: '333' }] })
    expect(await resolvePhoneNumberId('111', undefined, 'tok', fetchFn)).toBe('333')
    expect(calls[0]).toContain('/111/phone_numbers')
  })

  it('gives up when the WABA has several numbers or none', async () => {
    expect(await resolvePhoneNumberId('111', undefined, 'tok', fakeFetch({ data: [{ id: '1' }, { id: '2' }] }).fetchFn)).toBeNull()
    expect(await resolvePhoneNumberId('111', undefined, 'tok', fakeFetch({ data: [] }).fetchFn)).toBeNull()
    expect(await resolvePhoneNumberId('111', undefined, 'tok', fakeFetch({ error: { message: 'x' } }, false).fetchFn)).toBeNull()
  })
})

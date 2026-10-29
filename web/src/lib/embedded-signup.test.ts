import { describe, expect, it } from 'vitest'

import { parseSignupMessage } from '@/lib/embedded-signup'

const msg = (event: string, data: unknown = {}) =>
  JSON.stringify({ type: 'WA_EMBEDDED_SIGNUP', event, data })

describe('parseSignupMessage', () => {
  it('reads the ids from a normal FINISH', () => {
    expect(
      parseSignupMessage(msg('FINISH', { waba_id: '111', phone_number_id: '222' })),
    ).toEqual({ kind: 'finish', wabaId: '111', phoneNumberId: '222' })
  })

  it('treats the WhatsApp Business app (coexistence) finish as a finish', () => {
    expect(
      parseSignupMessage(
        msg('FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING', {
          waba_id: '111',
          phone_number_id: '222',
        }),
      ),
    ).toEqual({ kind: 'finish', wabaId: '111', phoneNumberId: '222' })
  })

  it('leaves the phone number id out when Meta does not send it', () => {
    // The server looks it up from the WABA. This is the normal shape of a
    // coexistence signup, not a malformed message.
    expect(
      parseSignupMessage(
        msg('FINISH_WHATSAPP_BUSINESS_APP_ONBOARDING', { waba_id: '111' }),
      ),
    ).toEqual({ kind: 'finish', wabaId: '111', phoneNumberId: undefined })
  })

  it('ignores a finish without a WABA id', () => {
    expect(parseSignupMessage(msg('FINISH', {}))).toBeNull()
  })

  it('reports cancel and error', () => {
    expect(parseSignupMessage(msg('CANCEL', { current_step: 'x' }))).toEqual({
      kind: 'cancel',
    })
    expect(parseSignupMessage(msg('ERROR', { error_message: 'boom' }))).toEqual({
      kind: 'error',
      message: 'boom',
    })
    expect(parseSignupMessage(msg('ERROR'))).toEqual({
      kind: 'error',
      message: 'Unknown error from Facebook',
    })
  })

  it('ignores other messages', () => {
    // The window receives postMessages from anything embedded on the page, so
    // anything that is not Meta's signup has to fall through silently.
    expect(parseSignupMessage('not json')).toBeNull()
    expect(
      parseSignupMessage(
        JSON.stringify({ type: 'OTHER', event: 'FINISH', data: { waba_id: '1' } }),
      ),
    ).toBeNull()
    expect(parseSignupMessage({ some: 'object' })).toBeNull()
  })
})

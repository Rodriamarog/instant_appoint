import { useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { call } from '@/lib/api'
import { CONFIG_ID, FB_APP_ID, parseSignupMessage } from '@/lib/embedded-signup'
import { errorMessage } from '@/lib/errors'

/** Meta only posts signup results from this origin. */
const FB_ORIGIN = 'https://www.facebook.com'

/**
 * Shown when the signup ends with no authorization code and Meta never said
 * why — the popup is simply gone. Deliberately says nothing about causes: the
 * person did not do this and cannot act on the reason, only on the retry.
 */
const SIGNUP_DID_NOT_FINISH =
  'The signup did not finish, so no number was connected. Try again.'

declare global {
  interface Window {
    fbAsyncInit: () => void
    FB?: {
      init: (opts: object) => void
      login: (cb: (response: FBResponse) => void, opts: object) => void
    }
  }
}

interface FBResponse {
  authResponse?: { code: string }
}

interface Props {
  /** Called once the server has stored the connection. */
  onConnected: () => void
}

type Status = 'idle' | 'loading' | 'success' | 'error'

/**
 * The Connect button.
 *
 * Meta's signup reports its result in two halves that arrive separately: the
 * popup posts the WhatsApp Business account id to this window, and the
 * FB.login callback returns an authorization code. Neither is useful alone, so
 * the posted half is held until the callback fires and both are sent to the
 * server together.
 */
export function WhatsAppEmbeddedSignup({ onConnected }: Props) {
  const [sdkReady, setSdkReady] = useState(false)
  const [status, setStatus] = useState<Status>('idle')
  const [failure, setFailure] = useState('')

  // A ref rather than state: the FB.login callback closes over its first
  // render, so a state value read there would be the one from before the
  // popup opened.
  const signup = useRef<{ wabaId: string; phoneNumberId?: string } | null>(null)

  // What the popup last reported, for the same reason signup is a ref: the
  // FB.login callback closes over its first render. Null means the popup
  // never said anything at all, which is the case worth complaining about.
  const popupOutcome = useRef<'cancel' | 'error' | null>(null)

  useEffect(() => {
    // A hot reload leaves the SDK loaded but fbAsyncInit already spent.
    if (window.FB) setSdkReady(true)

    window.fbAsyncInit = () => {
      window.FB?.init({
        appId: FB_APP_ID,
        autoLogAppEvents: true,
        xfbml: true,
        version: 'v25.0',
      })
      setSdkReady(true)
    }

    if (!document.getElementById('facebook-jssdk')) {
      const script = document.createElement('script')
      script.id = 'facebook-jssdk'
      script.src = 'https://connect.facebook.net/en_US/sdk.js'
      script.async = true
      script.defer = true
      script.onerror = () => {
        setStatus('error')
        setFailure('Could not load the Facebook SDK. An ad blocker usually causes this.')
      }
      document.body.appendChild(script)
    }

    const handleMessage = (event: MessageEvent) => {
      // The window hears from anything embedded in the page, so the origin
      // check is what makes the rest of this safe to trust.
      if (event.origin !== FB_ORIGIN) return

      const result = parseSignupMessage(event.data)
      if (!result) return

      if (result.kind === 'finish') {
        signup.current = { wabaId: result.wabaId, phoneNumberId: result.phoneNumberId }
      } else if (result.kind === 'cancel') {
        popupOutcome.current = 'cancel'
        setStatus('idle')
      } else {
        popupOutcome.current = 'error'
        setStatus('error')
        setFailure(result.message)
      }
    }

    window.addEventListener('message', handleMessage)

    return () => window.removeEventListener('message', handleMessage)
  }, [])

  const exchange = async (code: string) => {
    const finished = signup.current
    if (!finished) {
      setStatus('error')
      setFailure('Facebook did not say which WhatsApp Business account was connected.')
      return
    }

    try {
      await call(
        '/api/whatsapp/exchange-token',
        {
          method: 'POST',
          body: JSON.stringify({
            code,
            waba_id: finished.wabaId,
            // Absent for a business staying on the WhatsApp Business app; the
            // server looks the number up from the account instead.
            phone_number_id: finished.phoneNumberId,
          }),
        },
        'Connecting the account failed.',
      )

      signup.current = null
      setStatus('success')
      onConnected()
    } catch (err) {
      setStatus('error')
      setFailure(errorMessage(err, 'Connecting the account failed.'))
    }
  }

  const launch = () => {
    if (!window.FB) {
      setStatus('error')
      setFailure('The Facebook SDK is still loading. Try again in a moment.')
      return
    }

    setStatus('loading')
    setFailure('')
    signup.current = null
    popupOutcome.current = null

    window.FB.login(
      (response: FBResponse) => {
        // No authResponse covers three different endings, and they must
        // not look alike. Meta posts CANCEL when the popup was dismissed on
        // purpose and ERROR when it failed and said why; both have already
        // put the page where it belongs. Having heard neither, the attempt
        // died without a word — an SDK that lost track of the popup ends
        // exactly here — and staying quiet about that is what makes a broken
        // signup impossible to tell apart from one nobody attempted.
        if (!response.authResponse) {
          if (popupOutcome.current === null) {
            setStatus('error')
            setFailure(SIGNUP_DID_NOT_FINISH)
          } else if (popupOutcome.current === 'cancel') {
            setStatus('idle')
          }
          return
        }

        void exchange(response.authResponse.code)
      },
      {
        config_id: CONFIG_ID,
        response_type: 'code',
        override_default_response_type: true,
        extras: {
          version: 'v4',
          sessionInfoVersion: '3',
          featureType: 'whatsapp_business_app_onboarding',
          setup: {},
        },
      },
    )
  }

  return (
    <div className="space-y-3">
      <Button
        onClick={launch}
        disabled={!sdkReady || status === 'loading' || status === 'success'}
        className="bg-[#1877F2] text-white hover:bg-[#166FE5]"
      >
        {!sdkReady && 'Loading...'}
        {sdkReady && status === 'loading' && 'Opening Facebook...'}
        {sdkReady && status === 'success' && 'Connected'}
        {sdkReady && (status === 'idle' || status === 'error') && 'Connect WhatsApp Business'}
      </Button>

      {status === 'error' && failure && (
        <p className="text-sm text-destructive">{failure}</p>
      )}
    </div>
  )
}

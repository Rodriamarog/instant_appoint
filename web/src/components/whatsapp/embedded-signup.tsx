import { useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { parseSignupMessage } from '@/lib/embedded-signup'
import { pb } from '@/lib/pocketbase'

/**
 * The Meta app this signup runs against, and the Embedded Signup
 * configuration inside it. Both identify a public Facebook app rather than a
 * secret, which is why they can live in code the browser downloads; the app
 * secret never leaves the server.
 */
const FB_APP_ID = '1195277397801905'
const CONFIG_ID = '986479390379975'

/** Meta only posts signup results from this origin. */
const FB_ORIGIN = 'https://www.facebook.com'

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
  const [errorMessage, setErrorMessage] = useState('')

  // A ref rather than state: the FB.login callback closes over its first
  // render, so a state value read there would be the one from before the
  // popup opened.
  const signup = useRef<{ wabaId: string; phoneNumberId?: string } | null>(null)

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
        setErrorMessage('Could not load the Facebook SDK. An ad blocker usually causes this.')
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
        setStatus('idle')
      } else {
        setStatus('error')
        setErrorMessage(result.message)
      }
    }

    window.addEventListener('message', handleMessage)

    return () => window.removeEventListener('message', handleMessage)
  }, [])

  const exchange = async (code: string) => {
    const finished = signup.current
    if (!finished) {
      setStatus('error')
      setErrorMessage('Facebook did not say which WhatsApp Business account was connected.')
      return
    }

    try {
      const res = await fetch('/api/whatsapp/exchange-token', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${pb.authStore.token}`,
        },
        body: JSON.stringify({
          code,
          waba_id: finished.wabaId,
          // Absent for a business staying on the WhatsApp Business app; the
          // server looks the number up from the account instead.
          phone_number_id: finished.phoneNumberId,
        }),
      })

      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as { message?: string }
        throw new Error(body.message || 'Connecting the account failed.')
      }

      signup.current = null
      setStatus('success')
      onConnected()
    } catch (err) {
      setStatus('error')
      setErrorMessage(err instanceof Error ? err.message : 'Connecting the account failed.')
    }
  }

  const launch = () => {
    if (!window.FB) {
      setStatus('error')
      setErrorMessage('The Facebook SDK is still loading. Try again in a moment.')
      return
    }

    setStatus('loading')
    setErrorMessage('')
    signup.current = null

    window.FB.login(
      (response: FBResponse) => {
        // No authResponse means the popup was dismissed rather than failed.
        if (!response.authResponse) {
          setStatus('idle')
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

      {status === 'error' && errorMessage && (
        <p className="text-sm text-destructive">{errorMessage}</p>
      )}
    </div>
  )
}

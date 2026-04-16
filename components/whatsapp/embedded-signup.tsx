'use client'

import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { pb } from '@/lib/pocketbase'

const FB_APP_ID = '1195277397801905'
const CONFIG_ID = '986479390379975'

declare global {
  interface Window {
    fbAsyncInit: () => void
    FB: {
      init: (opts: object) => void
      login: (cb: (response: FBResponse) => void, opts: object) => void
    }
  }
}

interface FBResponse {
  authResponse?: { code: string }
}

interface Props {
  onSuccess: () => void
}

export default function WhatsAppEmbeddedSignup({ onSuccess }: Props) {
  const [sdkReady, setSdkReady] = useState(false)
  const [status, setStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  useEffect(() => {
    // If SDK already loaded (e.g. hot reload), mark ready immediately
    if (window.FB) {
      setSdkReady(true)
    }

    window.fbAsyncInit = function () {
      window.FB.init({
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
        setErrorMsg('Failed to load Facebook SDK. Try disabling ad blockers.')
        setStatus('error')
      }
      document.body.appendChild(script)
    }

    // Listen for embedded signup events (FINISH, CANCEL, ERROR)
    const handleMessage = (event: MessageEvent) => {
      if (event.origin !== 'https://www.facebook.com') return
      try {
        const data = JSON.parse(event.data)
        if (data.type !== 'WA_EMBEDDED_SIGNUP') return

        if (data.event === 'FINISH') {
          sessionStorage.setItem('wa_signup_data', JSON.stringify({
            waba_id: data.data.waba_id,
            phone_number_id: data.data.phone_number_id,
          }))
        } else if (data.event === 'CANCEL') {
          setStatus('idle')
        } else if (data.event === 'ERROR') {
          setStatus('error')
          setErrorMsg(data.data?.error_message ?? 'Unknown error from Facebook')
        }
      } catch {
        // non-JSON messages from Facebook, ignore
      }
    }

    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [])

  const launchWhatsAppSignup = () => {
    if (!window.FB) {
      setStatus('error')
      setErrorMsg('Facebook SDK not loaded yet. Please wait a moment and try again.')
      return
    }

    setStatus('loading')
    setErrorMsg(null)

    try {
      window.FB.login(
        async (response: FBResponse) => {
          if (!response.authResponse) {
            // User closed the popup without completing
            setStatus('idle')
            return
          }

          const code = response.authResponse.code
          const signupData = JSON.parse(sessionStorage.getItem('wa_signup_data') ?? '{}')

          try {
            const res = await fetch('/api/whatsapp/exchange-token', {
              method: 'POST',
              headers: {
                'Content-Type': 'application/json',
                Authorization: `Bearer ${pb.authStore.token}`,
              },
              body: JSON.stringify({
                code,
                waba_id: signupData.waba_id,
                phone_number_id: signupData.phone_number_id,
              }),
            })

            if (!res.ok) {
              const err = await res.json()
              throw new Error(err.error ?? 'Token exchange failed')
            }

            sessionStorage.removeItem('wa_signup_data')
            setStatus('success')
            onSuccess()
          } catch (err: unknown) {
            console.error(err)
            setStatus('error')
            setErrorMsg(err instanceof Error ? err.message : 'Failed to save WhatsApp Business account.')
          }
        },
        {
          config_id: CONFIG_ID,
          response_type: 'code',
          override_default_response_type: true,
          extras: {
            version: 'v4',
            setup: {},
          },
        }
      )
    } catch (err: unknown) {
      console.error(err)
      setStatus('error')
      setErrorMsg(err instanceof Error ? err.message : 'Failed to launch WhatsApp signup.')
    }
  }

  return (
    <div className="space-y-3">
      <Button
        onClick={launchWhatsAppSignup}
        disabled={status === 'loading' || status === 'success' || !sdkReady}
        className="bg-[#1877F2] hover:bg-[#166FE5] text-white"
      >
        {!sdkReady && 'Loading...'}
        {sdkReady && status === 'loading' && 'Opening Facebook...'}
        {sdkReady && status === 'success' && 'Connected!'}
        {sdkReady && (status === 'idle' || status === 'error') && 'Connect WhatsApp Business'}
      </Button>
      {status === 'error' && errorMsg && (
        <p className="text-sm text-red-600">{errorMsg}</p>
      )}
    </div>
  )
}

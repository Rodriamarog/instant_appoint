import { useEffect, useState } from 'react'

import { FB_APP_ID } from '@/lib/embedded-signup'

/**
 * Loads Facebook's JS SDK once for the whole app.
 *
 * Both the WhatsApp signup and the Messenger/Instagram login use it, and
 * both can be open on Integraciones at once. The SDK announces itself through
 * a single global, window.fbAsyncInit, so two components each setting their
 * own would have the second one's silently replace the first's — and the
 * first button would sit at "Cargando..." forever. One loader, many
 * listeners.
 */

export interface FBLoginResponse {
  authResponse?: { code: string }
}

declare global {
  interface Window {
    fbAsyncInit: () => void
    FB?: {
      init: (opts: object) => void
      login: (cb: (response: FBLoginResponse) => void, opts: object) => void
    }
  }
}

type State = 'loading' | 'ready' | 'failed'

let state: State = 'loading'
const listeners = new Set<(state: State) => void>()

function settle(next: State) {
  state = next
  listeners.forEach((listener) => listener(next))
}

function load() {
  // A hot reload leaves the SDK loaded but fbAsyncInit already spent.
  if (window.FB) {
    settle('ready')
    return
  }
  if (document.getElementById('facebook-jssdk')) {
    return
  }

  window.fbAsyncInit = () => {
    window.FB?.init({
      appId: FB_APP_ID,
      autoLogAppEvents: true,
      xfbml: true,
      version: 'v25.0',
    })
    settle('ready')
  }

  const script = document.createElement('script')
  script.id = 'facebook-jssdk'
  script.src = 'https://connect.facebook.net/en_US/sdk.js'
  script.async = true
  script.defer = true
  script.onerror = () => settle('failed')
  document.body.appendChild(script)
}

/** Whether the SDK is ready, still loading, or blocked from loading. */
export function useFacebookSdk(): State {
  const [current, setCurrent] = useState<State>(() => (window.FB ? 'ready' : state))

  useEffect(() => {
    listeners.add(setCurrent)
    load()
    setCurrent(window.FB ? 'ready' : state)

    return () => {
      listeners.delete(setCurrent)
    }
  }, [])

  return current
}

/** Shown when the SDK could not load at all. */
export const SDK_BLOCKED =
  'No se pudo cargar el SDK de Facebook. Normalmente lo provoca un bloqueador de anuncios.'

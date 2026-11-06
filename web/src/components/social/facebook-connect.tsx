import { useState } from 'react'
import { Facebook, Instagram } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { MESSAGING_CONFIG_ID } from '@/lib/embedded-signup'
import { errorMessage } from '@/lib/errors'
import { SDK_BLOCKED, useFacebookSdk, type FBLoginResponse } from '@/lib/facebook-sdk'
import {
  choosePage,
  connectWithCode,
  type Connected,
  type ConnectResult,
  type PageChoice,
} from '@/lib/social-accounts'

/** What the connection needs: list the Pages, read and send their messages, and the same for a linked Instagram account. */
const MESSAGING_SCOPES = [
  'pages_show_list',
  'pages_messaging',
  'pages_manage_metadata',
  'instagram_basic',
  'instagram_manage_messages',
  'business_management',
].join(',')

type Status =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'choosing'; pages: PageChoice[] }
  | { kind: 'error'; message: string }

/**
 * "Conectar con Facebook": one login that connects a Page for Messenger and,
 * when the Page has one linked, its Instagram professional account.
 *
 * The popup returns an authorization code and nothing else; the server turns
 * it into tokens. Somebody who manages several Pages is shown them here and
 * picks the clinic's.
 */
export function FacebookConnect({
  label = 'Conectar con Facebook',
  onConnected,
}: {
  label?: string
  onConnected: (connected: Connected) => void
}) {
  const sdk = useFacebookSdk()
  const [status, setStatus] = useState<Status>({ kind: 'idle' })

  const settle = (result: ConnectResult) => {
    if ('pages' in result) {
      setStatus({ kind: 'choosing', pages: result.pages })
      return
    }
    setStatus({ kind: 'idle' })
    onConnected(result.connected)
  }

  const fail = (err: unknown) =>
    setStatus({
      kind: 'error',
      message: errorMessage(err, 'No se pudo conectar con Facebook.'),
    })

  const launch = () => {
    if (!window.FB) return

    setStatus({ kind: 'loading' })
    window.FB.login(
      (response: FBLoginResponse) => {
        if (!response.authResponse?.code) {
          // Closed or declined: nothing happened, so nothing to report.
          setStatus({ kind: 'idle' })
          return
        }
        connectWithCode(response.authResponse.code).then(settle, fail)
      },
      {
        // A Login for Business configuration when one is set; otherwise the
        // classic login asking for the permissions by name (all already
        // approved on the app), which is how Neurocrow connected Pages before.
        ...(MESSAGING_CONFIG_ID ? { config_id: MESSAGING_CONFIG_ID } : { scope: MESSAGING_SCOPES }),
        response_type: 'code',
        override_default_response_type: true,
      },
    )
  }

  const pick = (pageId: string) => {
    setStatus({ kind: 'loading' })
    choosePage(pageId).then(settle, fail)
  }

  if (status.kind === 'choosing') {
    return (
      <div className="space-y-2">
        <p className="text-sm font-medium">¿Cuál es la página de tu negocio?</p>
        <ul className="divide-y divide-border overflow-hidden rounded-[10px] ring-1 ring-border">
          {status.pages.map((page) => (
            <li key={page.id}>
              <button
                type="button"
                onClick={() => pick(page.id)}
                className="flex w-full cursor-pointer items-center gap-3 px-3 py-2.5 text-left text-sm hover:bg-muted/60"
              >
                <Facebook className="size-4 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate font-medium">{page.name}</span>
                {page.instagram_username && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Instagram className="size-3.5" />@{page.instagram_username}
                  </span>
                )}
              </button>
            </li>
          ))}
        </ul>
        <Button variant="ghost" size="sm" onClick={() => setStatus({ kind: 'idle' })}>
          Cancelar
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <Button
        onClick={launch}
        disabled={sdk !== 'ready' || status.kind === 'loading'}
        className="bg-[#1877F2] text-white hover:bg-[#166FE5]"
      >
        <Facebook className="size-4" />
        {sdk === 'loading' ? 'Cargando...' : status.kind === 'loading' ? 'Conectando...' : label}
      </Button>
      {sdk === 'failed' && <p className="text-sm text-destructive">{SDK_BLOCKED}</p>}
      {status.kind === 'error' && <p className="text-sm text-destructive">{status.message}</p>}
    </div>
  )
}

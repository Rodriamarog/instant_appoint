import { useCallback, useEffect, useState } from 'react'

import { WhatsAppEmbeddedSignup } from '@/components/whatsapp/embedded-signup'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  displayNumber,
  listConnectedAccounts,
  type WhatsAppAccount,
} from '@/lib/whatsapp-accounts'

export function WhatsAppPage() {
  const [accounts, setAccounts] = useState<WhatsAppAccount[]>([])
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  const load = useCallback(async () => {
    try {
      setAccounts(await listConnectedAccounts())
      setError('')
    } catch {
      setError('Could not load your connected numbers.')
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const isConnected = accounts.length > 0

  return (
    <section className="space-y-5">
      <div>
        <h1 className="font-heading text-2xl font-semibold">WhatsApp</h1>
        <p className="text-muted-foreground">
          Connect the WhatsApp Business number appointment reminders and replies
          are sent from.
        </p>
      </div>

      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <CardTitle>WhatsApp Business API</CardTitle>
          <span
            className={
              isConnected
                ? 'rounded-full bg-primary px-3 py-1 text-xs font-medium text-primary-foreground'
                : 'rounded-full px-3 py-1 text-xs font-medium ring-1 ring-border'
            }
          >
            {isLoading ? 'Checking...' : isConnected ? 'Connected' : 'Not connected'}
          </span>
        </CardHeader>

        <CardContent className="space-y-4">
          {error && <p className="text-sm text-destructive">{error}</p>}

          {accounts.map((account) => (
            <div key={account.id} className="rounded-2xl bg-muted px-4 py-3">
              <p className="font-medium">{account.verified_name || 'WhatsApp Business'}</p>
              <p className="text-sm">{displayNumber(account)}</p>
              <p className="text-xs text-muted-foreground">
                WhatsApp Business account {account.waba_id}
              </p>
            </div>
          ))}

          <p className="text-sm text-muted-foreground">
            {isConnected
              ? 'This number is connected through Meta’s Cloud API. Connect another below.'
              : 'Your number must belong to a WhatsApp Business account. A number already on the free WhatsApp Business app can keep using it — Meta calls this coexistence, and the signup handles it.'}
          </p>

          <WhatsAppEmbeddedSignup onConnected={load} />
        </CardContent>
      </Card>
    </section>
  )
}

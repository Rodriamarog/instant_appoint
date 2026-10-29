import { useCallback, useEffect, useState } from 'react'

import { WhatsAppEmbeddedSignup } from '@/components/whatsapp/embedded-signup'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { pb } from '@/lib/pocketbase'

/**
 * A connected number, as the collection returns it.
 *
 * access_token is deliberately absent: it is a hidden field, so PocketBase
 * strips it from every response and answers a filter or sort that names one
 * with a 400. Nothing here may mention it.
 */
interface ConnectedAccount {
  id: string
  phone_number: string
  phone_number_id: string
  waba_id: string
  verified_name: string
}

/**
 * The number Meta assigned, preferred over the raw id when something stored
 * one. The Cloud API path never writes phone_number — only the retired QR
 * service did — so the fallback is the usual case.
 */
function displayNumber(account: ConnectedAccount) {
  return `+${account.phone_number || account.phone_number_id}`
}

export function WhatsAppPage() {
  const [accounts, setAccounts] = useState<ConnectedAccount[]>([])
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  const load = useCallback(async () => {
    try {
      const records = await pb
        .collection('whatsapp_accounts')
        .getFullList<ConnectedAccount>({
          // Ownership is enforced by the collection's list rule, which is the
          // only check a browser cannot talk its way around; naming the user
          // here as well keeps the query honest about what it expects back.
          filter: pb.filter(
            'user_id = {:userId} && account_type = "business_api" && is_active = true',
            { userId: pb.authStore.record?.id ?? '' },
          ),
          sort: '-created',
        })

      setAccounts(records)
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

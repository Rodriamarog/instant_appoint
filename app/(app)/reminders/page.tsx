'use client'

import { useState, useEffect } from 'react'
import { pb } from '@/lib/pocketbase'
import { RemindersTab } from '@/components/reminders/reminders-tab'

export default function RemindersPage() {
  const [whatsappStatus, setWhatsappStatus] = useState<string | null>(null)

  useEffect(() => {
    fetch('/api/whatsapp/status', {
      headers: { Authorization: `Bearer ${pb.authStore.token}` },
    })
      .then(r => r.ok ? r.json() : null)
      .then(d => setWhatsappStatus(d?.status ?? 'not_initialized'))
      .catch(() => setWhatsappStatus('not_initialized'))
  }, [])

  return <RemindersTab whatsappStatus={whatsappStatus} />
}

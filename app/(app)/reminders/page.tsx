'use client'

import { useState, useEffect } from 'react'
import { pb } from '@/lib/pocketbase'
import { RemindersTab } from '@/components/reminders/reminders-tab'

export default function RemindersPage() {
  const [whatsappStatus, setWhatsappStatus] = useState('not_initialized')

  useEffect(() => {
    fetch('/api/whatsapp/status', {
      headers: { Authorization: `Bearer ${pb.authStore.token}` },
    })
      .then(r => r.ok ? r.json() : null)
      .then(d => { if (d) setWhatsappStatus(d.status) })
      .catch(() => {})
  }, [])

  return <RemindersTab whatsappStatus={whatsappStatus} />
}

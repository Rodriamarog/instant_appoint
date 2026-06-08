'use client'

import { useState, useEffect } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Loader2, Plus, X } from 'lucide-react'
import { pb } from '@/lib/pocketbase'

interface Template {
  name: string
  status: 'APPROVED' | 'PENDING' | 'REJECTED' | 'PAUSED' | string
  category: string
  language: string
  components: { type: string; text?: string; example?: { body_text?: string[][] } }[]
}

const CATEGORIES = ['UTILITY', 'MARKETING', 'AUTHENTICATION']
const LANGUAGES = [
  { code: 'es', label: 'Spanish (es)' },
  { code: 'en_US', label: 'English (en_US)' },
  { code: 'pt_BR', label: 'Portuguese (pt_BR)' },
]

export default function TemplatesPage() {
  const [templates, setTemplates] = useState<Template[]>([])
  const [loading, setLoading] = useState(true)
  const [hasAccount, setHasAccount] = useState(false)

  // Create form
  const [showCreate, setShowCreate] = useState(false)
  const [newName, setNewName] = useState('')
  const [newBody, setNewBody] = useState('')
  const [newCategory, setNewCategory] = useState('UTILITY')
  const [newLanguage, setNewLanguage] = useState('es')
  const [creating, setCreating] = useState(false)
  const [createFeedback, setCreateFeedback] = useState<{ ok: boolean; msg: string } | null>(null)

  // Send test
  const [sendTarget, setSendTarget] = useState<{ name: string; lang: string } | null>(null)
  const [testPhone, setTestPhone] = useState('16197612314')
  const [sending, setSending] = useState(false)
  const [sendFeedback, setSendFeedback] = useState<{ ok: boolean; msg: string; templateName: string } | null>(null)

  useEffect(() => { loadAll() }, [])

  const authHeaders = () => ({
    'Content-Type': 'application/json',
    Authorization: `Bearer ${pb.authStore.token}`,
  })

  const loadAll = async () => {
    setLoading(true)
    try {
      const res = await fetch('/api/whatsapp/status', { headers: authHeaders() })
      if (!res.ok) return
      const data = await res.json()
      const accounts = data.cloudAccounts ?? []
      setHasAccount(accounts.length > 0)
      if (accounts.length > 0) await loadTemplates()
    } finally {
      setLoading(false)
    }
  }

  const loadTemplates = async () => {
    const res = await fetch('/api/whatsapp/templates', { headers: authHeaders() })
    const data = await res.json()
    if (!res.ok) return
    setTemplates(data.templates ?? [])
  }

  const createTemplate = async () => {
    if (!newName || !newBody) return
    setCreating(true)
    setCreateFeedback(null)
    try {
      const res = await fetch('/api/whatsapp/templates/create', {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify({ name: newName, category: newCategory, language: newLanguage, body: newBody }),
      })
      const data = await res.json()
      if (res.ok) {
        setCreateFeedback({ ok: true, msg: 'Template submitted — it will appear as PENDING until Meta approves it.' })
        setNewName('')
        setNewBody('')
        setShowCreate(false)
        await loadTemplates()
      } else {
        setCreateFeedback({ ok: false, msg: data.error ?? 'Failed to create template' })
      }
    } catch {
      setCreateFeedback({ ok: false, msg: 'Network error' })
    } finally {
      setCreating(false)
    }
  }

  const sendTemplate = async () => {
    if (!sendTarget || !testPhone) return
    setSending(true)
    setSendFeedback(null)
    const name = sendTarget.name
    try {
      const res = await fetch('/api/whatsapp/templates/send', {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify({ template_name: name, language_code: sendTarget.lang, to: testPhone }),
      })
      const data = await res.json()
      if (res.ok) {
        setSendFeedback({ ok: true, msg: 'Message sent successfully!', templateName: name })
        setSendTarget(null)
        setTestPhone('')
      } else {
        setSendFeedback({ ok: false, msg: data.error ?? 'Failed to send', templateName: name })
      }
    } catch {
      setSendFeedback({ ok: false, msg: 'Network error', templateName: name })
    } finally {
      setSending(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-gray-500">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading...
      </div>
    )
  }

  if (!hasAccount) {
    return (
      <div className="space-y-2">
        <h2 className="text-2xl font-bold text-gray-900">Templates</h2>
        <p className="text-sm text-gray-500">
          Connect a WhatsApp Business account first in{' '}
          <a href="/whatsapp" className="underline">WhatsApp Setup</a>.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Templates</h2>
          <p className="text-gray-600 text-sm mt-0.5">Create and send WhatsApp message templates.</p>
        </div>
        <Button
          onClick={() => { setShowCreate(v => !v); setCreateFeedback(null) }}
          className="bg-black text-white hover:bg-gray-800 flex items-center gap-1.5"
        >
          {showCreate ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
          {showCreate ? 'Cancel' : 'New template'}
        </Button>
      </div>

      {/* Create form */}
      {showCreate && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">New template</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid grid-cols-3 gap-3">
              <div className="space-y-1">
                <label className="text-xs font-medium text-gray-700">Name</label>
                <Input
                  placeholder="e.g. appointment_reminder"
                  value={newName}
                  onChange={e => setNewName(e.target.value.toLowerCase().replace(/\s+/g, '_').replace(/[^a-z0-9_]/g, ''))}
                  className="text-sm h-8"
                />
                <p className="text-xs text-gray-400">Lowercase letters, numbers, underscores only</p>
              </div>
              <div className="space-y-1">
                <label className="text-xs font-medium text-gray-700">Category</label>
                <select
                  value={newCategory}
                  onChange={e => setNewCategory(e.target.value)}
                  className="w-full text-sm h-8 border border-gray-200 rounded-md px-2"
                >
                  {CATEGORIES.map(c => <option key={c} value={c}>{c}</option>)}
                </select>
              </div>
              <div className="space-y-1">
                <label className="text-xs font-medium text-gray-700">Language</label>
                <select
                  value={newLanguage}
                  onChange={e => setNewLanguage(e.target.value)}
                  className="w-full text-sm h-8 border border-gray-200 rounded-md px-2"
                >
                  {LANGUAGES.map(l => <option key={l.code} value={l.code}>{l.label}</option>)}
                </select>
              </div>
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium text-gray-700">Body</label>
              <Textarea
                placeholder="Your appointment is confirmed for {{1}}. See you then!"
                value={newBody}
                onChange={e => setNewBody(e.target.value)}
                className="text-sm resize-none"
                rows={3}
              />
            </div>
            <div className="flex items-center gap-3">
              <Button
                onClick={createTemplate}
                disabled={creating || !newName || !newBody}
                className="bg-black text-white hover:bg-gray-800 h-8 px-4 text-sm"
              >
                {creating ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Submit for approval'}
              </Button>
              {createFeedback && (
                <p className={`text-sm ${createFeedback.ok ? 'text-green-600' : 'text-red-600'}`}>
                  {createFeedback.msg}
                </p>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Template list */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-base">Your templates</CardTitle>
            <Button variant="outline" className="text-sm px-3 py-1.5 h-auto" onClick={loadTemplates}>
              Refresh
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {templates.length === 0 && (
            <p className="text-sm text-gray-500">No templates yet. Create one above.</p>
          )}
          {templates.map(t => {
            const body = t.components.find(c => c.type === 'BODY')?.text ?? ''
            const isApproved = t.status === 'APPROVED'
            const isOpen = sendTarget?.name === t.name && sendTarget?.lang === t.language
            return (
              <div key={t.name + t.language} className="border border-gray-200 rounded-lg p-3 space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="font-medium text-sm truncate">{t.name}</span>
                    <span className="text-xs text-gray-400 shrink-0">{t.language}</span>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Badge variant={isApproved ? 'default' : t.status === 'PENDING' ? 'secondary' : 'destructive'}>
                      {t.status}
                    </Badge>
                    <span className="text-xs text-gray-400">{t.category}</span>
                    {isApproved && (
                      <Button
                        className="text-xs px-2 py-1 h-auto bg-black text-white hover:bg-gray-800"
                        onClick={() => {
                          if (isOpen) { setSendTarget(null); setSendFeedback(null); return }
                          setSendTarget({ name: t.name, lang: t.language })
                          setSendFeedback(null)
                        }}
                      >
                        {isOpen ? 'Cancel' : 'Send test'}
                      </Button>
                    )}
                  </div>
                </div>
                {body && <p className="text-xs text-gray-500 bg-gray-50 rounded p-2">{body}</p>}
                {isOpen && (
                  <div className="flex gap-2 pt-1">
                    <Input
                      placeholder="Phone number (e.g. 16197612314)"
                      value={testPhone}
                      onChange={e => setTestPhone(e.target.value)}
                      className="text-sm h-8"
                    />
                    <Button
                      onClick={sendTemplate}
                      disabled={sending || !testPhone}
                      className="h-8 px-3 text-sm bg-black text-white hover:bg-gray-800 shrink-0"
                    >
                      {sending ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Send'}
                    </Button>
                  </div>
                )}
                {sendFeedback?.templateName === t.name && (
                  <p className={`text-sm ${sendFeedback.ok ? 'text-green-600' : 'text-red-600'}`}>
                    {sendFeedback.msg}
                  </p>
                )}
              </div>
            )
          })}
        </CardContent>
      </Card>
    </div>
  )
}

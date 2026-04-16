'use client'

import { useState, useEffect, useRef } from 'react'
import { pb } from '@/lib/pocketbase'

interface Conversation {
  id: string
  customer_phone: string
  status: string
  last_message_at: string
}

interface Message {
  id: string
  direction: 'inbound' | 'outbound'
  message_content: string
  sent_at: string
}

export default function ChatPage() {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [selectedConv, setSelectedConv] = useState<Conversation | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const bottomRef = useRef<HTMLDivElement>(null)

  // Load conversations
  useEffect(() => {
    loadConversations()
    const interval = setInterval(loadConversations, 5000)
    return () => clearInterval(interval)
  }, [])

  // Load messages when conversation selected, poll for new ones
  useEffect(() => {
    if (!selectedConv) return
    loadMessages(selectedConv.id)
    const interval = setInterval(() => loadMessages(selectedConv.id), 3000)
    return () => clearInterval(interval)
  }, [selectedConv?.id])

  // Scroll to bottom on new messages
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const loadConversations = async () => {
    try {
      const userId = pb.authStore.model?.id
      const res = await fetch(
        `/api/chat/conversations`,
        { headers: { Authorization: `Bearer ${pb.authStore.token}` } }
      )
      if (res.ok) {
        const data = await res.json()
        setConversations(data.conversations ?? [])
      }
    } catch {}
  }

  const loadMessages = async (convId: string) => {
    try {
      const res = await fetch(
        `/api/chat/messages?conversation_id=${convId}`,
        { headers: { Authorization: `Bearer ${pb.authStore.token}` } }
      )
      if (res.ok) {
        const data = await res.json()
        setMessages(data.messages ?? [])
      }
    } catch {}
  }

  const sendMessage = async () => {
    if (!input.trim() || !selectedConv || sending) return
    setSending(true)
    try {
      const res = await fetch('/api/chat/send', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${pb.authStore.token}`,
        },
        body: JSON.stringify({ conversation_id: selectedConv.id, message: input.trim() }),
      })
      if (res.ok) {
        setInput('')
        await loadMessages(selectedConv.id)
      } else {
        const err = await res.json()
        alert(err.error ?? 'Failed to send')
      }
    } catch {
      alert('Failed to send message')
    } finally {
      setSending(false)
    }
  }

  const formatTime = (iso: string) => {
    if (!iso) return ''
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }

  const formatPhone = (phone: string) => `+${phone}`

  return (
    <div>
      <h2 className="text-2xl font-bold text-gray-900 mb-5">Chat</h2>
      <div className="border border-gray-200 rounded-xl overflow-hidden flex" style={{ height: '70vh' }}>

        {/* Sidebar */}
        <div className="w-72 border-r border-gray-200 flex flex-col flex-shrink-0">
          <div className="px-4 py-3 border-b border-gray-100 bg-gray-50">
            <p className="text-sm font-medium text-gray-700">Conversations</p>
          </div>
          <div className="overflow-y-auto flex-1">
            {conversations.length === 0 && (
              <p className="text-sm text-gray-400 text-center mt-8">No conversations yet</p>
            )}
            {conversations.map(conv => (
              <button
                key={conv.id}
                onClick={() => setSelectedConv(conv)}
                className={`w-full text-left px-4 py-3 border-b border-gray-100 hover:bg-gray-50 transition-colors ${
                  selectedConv?.id === conv.id ? 'bg-gray-100' : ''
                }`}
              >
                <p className="text-sm font-medium text-gray-900">{formatPhone(conv.customer_phone)}</p>
                <p className="text-xs text-gray-400 mt-0.5">{formatTime(conv.last_message_at)}</p>
              </button>
            ))}
          </div>
        </div>

        {/* Chat area */}
        {!selectedConv ? (
          <div className="flex-1 flex items-center justify-center text-gray-400 text-sm">
            Select a conversation
          </div>
        ) : (
          <div className="flex-1 flex flex-col">
            {/* Header */}
            <div className="px-4 py-3 border-b border-gray-100 bg-gray-50">
              <p className="text-sm font-semibold text-gray-900">{formatPhone(selectedConv.customer_phone)}</p>
            </div>

            {/* Messages */}
            <div className="flex-1 overflow-y-auto px-4 py-4 space-y-2">
              {messages.map(msg => (
                <div
                  key={msg.id}
                  className={`flex ${msg.direction === 'outbound' ? 'justify-end' : 'justify-start'}`}
                >
                  <div
                    className={`max-w-xs lg:max-w-md px-3 py-2 rounded-2xl text-sm ${
                      msg.direction === 'outbound'
                        ? 'bg-black text-white rounded-br-sm'
                        : 'bg-gray-100 text-gray-900 rounded-bl-sm'
                    }`}
                  >
                    <p>{msg.message_content}</p>
                    <p className={`text-xs mt-1 ${msg.direction === 'outbound' ? 'text-gray-400' : 'text-gray-400'}`}>
                      {formatTime(msg.sent_at)}
                    </p>
                  </div>
                </div>
              ))}
              <div ref={bottomRef} />
            </div>

            {/* Input */}
            <div className="px-4 py-3 border-t border-gray-100 flex gap-2">
              <input
                type="text"
                value={input}
                onChange={e => setInput(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && !e.shiftKey && sendMessage()}
                placeholder="Type a message..."
                className="flex-1 border border-gray-200 rounded-full px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-black"
              />
              <button
                onClick={sendMessage}
                disabled={sending || !input.trim()}
                className="bg-black text-white px-4 py-2 rounded-full text-sm font-medium hover:bg-gray-800 disabled:opacity-40 transition-colors"
              >
                {sending ? '...' : 'Send'}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

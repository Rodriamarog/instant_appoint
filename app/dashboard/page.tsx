'use client'

import { useState, useEffect, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import dynamic from 'next/dynamic'
import { momentLocalizer } from 'react-big-calendar'
import type { View, CalendarProps } from 'react-big-calendar'
import moment from 'moment'
import 'react-big-calendar/lib/css/react-big-calendar.css'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Loader2 } from 'lucide-react'
import { pb } from '@/lib/pocketbase'
import { WhatsAppClient } from '@/lib/whatsapp-client'
import { EventDialog } from '@/components/calendar/event-dialog'
import { RemindersTab } from '@/components/reminders/reminders-tab'

const localizer = momentLocalizer(moment)

interface CalendarEvent {
  id: string
  title: string
  client_phone?: string
  notes?: string
  start_time: string
  end_time: string
  location?: string
}

interface BigCalendarEvent {
  id: string
  title: string
  start: Date
  end: Date
  resource: CalendarEvent
}

const BigCalendar = dynamic<CalendarProps<BigCalendarEvent, CalendarEvent>>(
  () => import('react-big-calendar').then((mod) => mod.Calendar as any),
  { loading: () => <div className="flex items-center justify-center h-full"><Loader2 className="h-8 w-8 animate-spin" /></div>, ssr: false }
)

type TabType = 'calendar' | 'reminders' | 'whatsapp'

export default function DashboardPage() {
  const [user, setUser] = useState<any>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<TabType>('calendar')
  const router = useRouter()

  // WhatsApp states
  const [whatsappClient] = useState(() => new WhatsAppClient())
  const [whatsappStatus, setWhatsappStatus] = useState('not_initialized')
  const [qrCode, setQrCode] = useState<string | null>(null)
  const [connectedNumber, setConnectedNumber] = useState<string | null>(null)
  const [isConnecting, setIsConnecting] = useState(false)
  // Calendar states
  const [calendarEvents, setCalendarEvents] = useState<BigCalendarEvent[]>([])
  const [calendarDate, setCalendarDate] = useState(new Date())
  const [calendarView, setCalendarView] = useState<View>('month')
  const [selectedEvent, setSelectedEvent] = useState<CalendarEvent | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [dialogMode, setDialogMode] = useState<'create' | 'edit'>('create')

  const [testMessage, setTestMessage] = useState('')
  const [testNumber, setTestNumber] = useState('')
  const [testCountryCode, setTestCountryCode] = useState('1')
  const [isSendingTest, setIsSendingTest] = useState(false)

  const COUNTRY_CODES = [
    { label: '+1', value: '1', flag: '🇺🇸' },
    // WhatsApp requires 521 (not 52) for Mexican mobile numbers
    { label: '+52', value: '521', flag: '🇲🇽' },
  ]

  useEffect(() => {
    const loadData = async () => {
      try {
        // Only access auth store on client side
        if (typeof window === 'undefined') return

        console.log('Dashboard loading... Auth store valid:', pb.authStore.isValid)
        console.log('Auth store model:', pb.authStore.model)

        if (!pb.authStore.isValid) {
          console.log('Not authenticated, redirecting to login')
          router.push('/login')
          return
        }

        console.log('Setting user data')
        const userData = pb.authStore.model
        setUser(userData)

        // Initialize WhatsApp client with user ID
        if (userData?.id) {
          initializeWhatsApp(userData.id)
        }
      } catch (error) {
        console.error('Error loading dashboard data:', error)
      } finally {
        setIsLoading(false)
      }
    }

    loadData()
  }, [router])

  const initializeWhatsApp = (userId: string) => {
    console.log('Initializing WhatsApp for user ID:', userId)

    // Connect to WhatsApp service
    const socket = whatsappClient.connect(userId)

    console.log('Socket connected:', !!socket)

    // Set up event listeners
    whatsappClient.onQRUpdate((data) => {
      console.log('QR Update received:', data)
      setQrCode(data.qrCode)
      setWhatsappStatus(data.status)
    })

    whatsappClient.onConnectionUpdate((data) => {
      setWhatsappStatus(data.status)
      if (data.connectedNumber) {
        setConnectedNumber(data.connectedNumber)
        setQrCode(null) // Clear QR code when connected
      }
      if (data.status === 'disconnected') {
        setConnectedNumber(null)
        setQrCode(null)
      }
      setIsConnecting(false)
    })

    // Get initial status
    loadWhatsAppStatus()
  }

  const loadCalendarEvents = useCallback(async () => {
    const startDate = moment(calendarDate).startOf('month').subtract(7, 'days').toISOString()
    const endDate = moment(calendarDate).endOf('month').add(7, 'days').toISOString()
    try {
      const response = await fetch(
        `/api/calendar/events?start_date=${encodeURIComponent(startDate)}&end_date=${encodeURIComponent(endDate)}`,
        { headers: { Authorization: `Bearer ${pb.authStore.token}` } }
      )
      if (!response.ok) {
        const err = await response.json().catch(() => ({}))
        console.error('Failed to load calendar events:', response.status, err)
        return
      }
      const data = await response.json()
      const converted: BigCalendarEvent[] = (data.events || []).map((e: CalendarEvent) => ({
        id: e.id,
        title: e.title,
        start: new Date(e.start_time),
        end: new Date(e.end_time),
        resource: e,
      }))
      setCalendarEvents(converted)
    } catch (err) {
      console.error('Error loading calendar events:', err)
    }
  }, [calendarDate])

  useEffect(() => {
    if (activeTab === 'calendar') loadCalendarEvents()
  }, [calendarDate, activeTab, loadCalendarEvents])

  const loadWhatsAppStatus = async () => {
    try {
      const status = await whatsappClient.getStatus()
      setWhatsappStatus(status.status)
      setConnectedNumber(status.connectedNumber)
    } catch (error) {
      console.error('Error loading WhatsApp status:', error)
      setWhatsappStatus('not_initialized')
    }
  }

  const startWhatsAppConnection = async () => {
    setIsConnecting(true)
    setQrCode(null)
    try {
      await whatsappClient.initSession()
      setWhatsappStatus('waiting_qr')
    } catch (error) {
      console.error('Error starting WhatsApp connection:', error)
      setWhatsappStatus('error')
    } finally {
      setIsConnecting(false)
    }
  }

  const disconnectWhatsApp = async () => {
    try {
      await whatsappClient.disconnectSession()
      setWhatsappStatus('disconnected')
      setConnectedNumber(null)
      setQrCode(null)
    } catch (error) {
      console.error('Error disconnecting WhatsApp:', error)
    }
  }

  const sendTestMessage = async () => {
    if (!testNumber || !testMessage) return

    setIsSendingTest(true)
    try {
      await whatsappClient.sendMessage(`${testCountryCode}${testNumber}`, testMessage)
      setTestMessage('')
      alert('Test message sent successfully!')
    } catch (error) {
      console.error('Error sending test message:', error)
      alert('Failed to send test message')
    } finally {
      setIsSendingTest(false)
    }
  }

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      whatsappClient.disconnect()
    }
  }, [])

  const handleLogout = async () => {
    pb.authStore.clear()
    router.push('/login')
  }

  const tabs = [
    { id: 'calendar' as TabType, label: 'Calendar' },
    { id: 'reminders' as TabType, label: 'Reminders' },
    { id: 'whatsapp' as TabType, label: 'WhatsApp Setup' }
  ]

  const renderTabContent = () => {
    switch (activeTab) {
      case 'calendar':
        return (
          <div style={{ height: 'calc(100vh - 120px)' }}>
            <BigCalendar
              localizer={localizer}
              events={calendarEvents}
              startAccessor="start"
              endAccessor="end"
              view={calendarView}
              onView={setCalendarView}
              date={calendarDate}
              onNavigate={setCalendarDate}
              onSelectSlot={(slotInfo) => {
                setSelectedEvent({ id: '', title: '', start_time: slotInfo.start.toISOString(), end_time: slotInfo.end.toISOString() })
                setDialogMode('create')
                setDialogOpen(true)
              }}
              onSelectEvent={(event) => {
                setSelectedEvent(event.resource)
                setDialogMode('edit')
                setDialogOpen(true)
              }}
              selectable
              popup
              step={30}
              showMultiDayTimes
              style={{ height: '100%' }}
            />
            <EventDialog
              open={dialogOpen}
              mode={dialogMode}
              event={selectedEvent}
              onClose={() => setDialogOpen(false)}
              onSaved={() => { setDialogOpen(false); loadCalendarEvents() }}
            />
          </div>
        )
      case 'reminders':
        return <RemindersTab whatsappStatus={whatsappStatus} />
      case 'whatsapp':
        return (
          <div className="space-y-5">
            <div>
              <h2 className="text-2xl font-bold text-gray-900 mb-2">WhatsApp Integration</h2>
              <p className="text-gray-600">
                Connect your WhatsApp account to send appointment notifications.
              </p>
            </div>

            {/* Connection Status */}
            <Card>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <CardTitle>Connection Status</CardTitle>
                  <Badge variant={
                    whatsappStatus === 'connected' ? 'default' :
                    whatsappStatus === 'waiting_qr' || whatsappStatus === 'connecting' || whatsappStatus === 'initializing' ? 'secondary' :
                    whatsappStatus === 'error' ? 'destructive' : 'outline'
                  }>
                    {whatsappStatus === 'connected' && 'Connected'}
                    {whatsappStatus === 'waiting_qr' && 'Waiting for QR scan'}
                    {whatsappStatus === 'connecting' && 'Connecting...'}
                    {whatsappStatus === 'initializing' && (
                      <div className="flex items-center gap-1">
                        <Loader2 className="h-3 w-3 animate-spin" />
                        Generating QR code...
                      </div>
                    )}
                    {whatsappStatus === 'disconnected' && 'Disconnected'}
                    {whatsappStatus === 'not_initialized' && 'Not connected'}
                    {whatsappStatus === 'error' && 'Error'}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                {connectedNumber && (
                  <div className="p-3 bg-green-50 border border-green-200 rounded-lg">
                    <p className="text-sm text-green-800">
                      <strong>Connected Number:</strong> +{connectedNumber}
                    </p>
                  </div>
                )}

                <div className="flex gap-3">
                  {whatsappStatus === 'connected' ? (
                    <Button
                      onClick={disconnectWhatsApp}
                      className="px-4 py-2 text-sm bg-black text-white hover:bg-gray-800"
                    >
                      Disconnect WhatsApp
                    </Button>
                  ) : (
                    <Button
                      onClick={startWhatsAppConnection}
                      disabled={isConnecting}
                      className="px-4 py-2 text-sm bg-black text-white hover:bg-gray-800"
                    >
                      {isConnecting ? 'Starting...' :
                       whatsappStatus === 'waiting_qr' ? 'Restart WhatsApp' : 'Connect WhatsApp'}
                    </Button>
                  )}

                  <Button
                    onClick={loadWhatsAppStatus}
                    variant="outline"
                    className="px-4 py-2 text-sm"
                  >
                    Refresh Status
                  </Button>
                </div>
              </CardContent>
            </Card>

            {/* QR Code Display */}
            {(qrCode || whatsappStatus === 'initializing' || whatsappStatus === 'waiting_qr') && (
              <Card>
                <CardHeader>
                  <CardTitle>Scan QR Code</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-center space-y-4">
                    <p className="text-sm text-gray-600">
                      Open WhatsApp on your phone, go to Settings → Linked Devices → Link a Device, and scan this QR code:
                    </p>
                    <div className="flex justify-center">
                      {qrCode ? (
                        <img
                          src={qrCode}
                          alt="WhatsApp QR Code"
                          className="w-64 h-64 border border-gray-200 rounded-lg"
                        />
                      ) : (
                        <div className="w-64 h-64 border border-gray-200 rounded-lg flex items-center justify-center bg-gray-50">
                          <div className="text-center space-y-2">
                            <Loader2 className="h-8 w-8 animate-spin mx-auto text-gray-400" />
                            <p className="text-sm text-gray-500">Generating QR code...</p>
                          </div>
                        </div>
                      )}
                    </div>
                    <p className="text-xs text-gray-500">
                      QR code refreshes every 30 seconds
                    </p>
                  </div>
                </CardContent>
              </Card>
            )}

            {/* Test Message */}
            {whatsappStatus === 'connected' && (
              <Card>
                <CardHeader>
                  <CardTitle>Send Test Message</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="space-y-3">
                    <div>
                      <label htmlFor="testNumber" className="block text-sm font-medium text-gray-700 mb-1">
                        Phone Number
                      </label>
                      <div className="flex gap-2">
                        <select
                          value={testCountryCode}
                          onChange={(e) => setTestCountryCode(e.target.value)}
                          className="border border-gray-300 rounded-md px-2 py-2 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-black"
                        >
                          {COUNTRY_CODES.map((c) => (
                            <option key={c.value} value={c.value}>
                              {c.flag} {c.label}
                            </option>
                          ))}
                        </select>
                        <Input
                          id="testNumber"
                          type="text"
                          placeholder="5512345678"
                          value={testNumber}
                          onChange={(e) => setTestNumber(e.target.value)}
                          className="flex-1"
                        />
                      </div>
                    </div>
                    <div>
                      <label htmlFor="testMessage" className="block text-sm font-medium text-gray-700 mb-1">
                        Message
                      </label>
                      <Input
                        id="testMessage"
                        type="text"
                        placeholder="Test message from InstantAppoint"
                        value={testMessage}
                        onChange={(e) => setTestMessage(e.target.value)}
                      />
                    </div>
                    <Button
                      onClick={sendTestMessage}
                      disabled={isSendingTest || !testNumber || !testMessage}
                      className="w-full bg-black text-white hover:bg-gray-800"
                    >
                      {isSendingTest ? 'Sending...' : 'Send Test Message'}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>
        )
      default:
        return null
    }
  }

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900"></div>
      </div>
    )
  }

  return (
    <div className="bg-white min-h-screen">
      {/* Navigation */}
      <div className="bg-white border-b border-gray-100">
        <div className="max-w-5xl mx-auto px-4">
          <div className="relative flex justify-center items-center border-b border-gray-100">
            <div className="flex space-x-8">
              {tabs.map((tab) => (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id)}
                  className={`py-4 px-2 text-sm font-medium transition-colors whitespace-nowrap border-b-2 ${
                    activeTab === tab.id
                      ? 'border-black text-gray-900'
                      : 'border-transparent text-gray-600 hover:text-gray-900'
                  }`}
                >
                  {tab.label}
                </button>
              ))}
            </div>
            <Button
              onClick={handleLogout}
              className="absolute right-0 bg-black text-white px-3 py-2 text-sm font-medium rounded-lg hover:bg-gray-800 transition-colors"
            >
              Sign Out
            </Button>
          </div>
        </div>
      </div>

      {/* Tab Content */}
      <div className="max-w-5xl mx-auto px-4 py-8">
        {renderTabContent()}
      </div>
    </div>
  )
}
'use client'

import { useEffect } from 'react'
import { useRouter, usePathname } from 'next/navigation'
import Link from 'next/link'
import { pb } from '@/lib/pocketbase'

const NAV_LINKS = [
  { href: '/calendar',  label: 'Calendar' },
  { href: '/reminders', label: 'Reminders' },
  { href: '/whatsapp',  label: 'WhatsApp Setup' },
  { href: '/chat',      label: 'Chat' },
]

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const pathname = usePathname()

  useEffect(() => {
    if (typeof window === 'undefined') return
    if (!pb.authStore.isValid) router.push('/login')
  }, [router])

  const handleLogout = () => {
    pb.authStore.clear()
    router.push('/login')
  }

  return (
    <div className="bg-white min-h-screen">
      <div className="bg-white border-b border-gray-100">
        <div className="max-w-5xl mx-auto px-4">
          <div className="relative flex justify-center items-center">
            <div className="flex space-x-8">
              {NAV_LINKS.map(({ href, label }) => (
                <Link
                  key={href}
                  href={href}
                  className={`py-4 px-2 text-sm font-medium transition-colors whitespace-nowrap border-b-2 ${
                    pathname === href
                      ? 'border-black text-gray-900'
                      : 'border-transparent text-gray-600 hover:text-gray-900'
                  }`}
                >
                  {label}
                </Link>
              ))}
            </div>
            <button
              onClick={handleLogout}
              className="absolute right-0 bg-black text-white px-3 py-2 text-sm font-medium rounded-lg hover:bg-gray-800 transition-colors"
            >
              Sign Out
            </button>
          </div>
        </div>
      </div>
      <div className="max-w-5xl mx-auto px-4 py-8">
        {children}
      </div>
    </div>
  )
}

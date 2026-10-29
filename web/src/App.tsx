import { Route, Routes } from 'react-router'

import { RequireAuth } from '@/components/require-auth'
import { Shell } from '@/components/shell'
import { CalendarPage } from '@/pages/calendar'
import { HomePage } from '@/pages/home'
import { LoginPage } from '@/pages/login'
import { NotFoundPage } from '@/pages/not-found'
import { RegisterPage } from '@/pages/register'
import { TemplatesPage } from '@/pages/templates'
import { WhatsAppPage } from '@/pages/whatsapp'

/**
 * The route table. Tickets #3-#8 replace the placeholder pages and add the
 * rest of the app routes (chat, reminders, templates, whatsapp) under the
 * guard, plus the two public legal pages.
 */
export function App() {
  return (
    <Routes>
      <Route index element={<HomePage />} />
      <Route path="login" element={<LoginPage />} />
      <Route path="register" element={<RegisterPage />} />

      {/* Everything below here needs a session. */}
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route path="calendar" element={<CalendarPage />} />
          <Route path="whatsapp" element={<WhatsAppPage />} />
          <Route path="templates" element={<TemplatesPage />} />
        </Route>
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}

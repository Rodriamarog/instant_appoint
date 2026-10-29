import { Route, Routes } from 'react-router'

import { RequireAuth } from '@/components/require-auth'
import { Shell } from '@/components/shell'
import { CalendarPage } from '@/pages/calendar'
import { ChatPage } from '@/pages/chat'
import { HomePage } from '@/pages/home'
import { LoginPage } from '@/pages/login'
import { NotFoundPage } from '@/pages/not-found'
import { PrivacyPolicyPage } from '@/pages/privacy-policy'
import { RegisterPage } from '@/pages/register'
import { RemindersPage } from '@/pages/reminders'
import { TemplatesPage } from '@/pages/templates'
import { TermsOfServicePage } from '@/pages/terms-of-service'
import { WhatsAppPage } from '@/pages/whatsapp'

/**
 * The route table.
 *
 * The two legal pages are deliberately public and outside the shell: Meta's
 * app review fetches them signed out, so putting them behind the auth guard
 * would fail it.
 */
export function App() {
  return (
    <Routes>
      <Route index element={<HomePage />} />
      <Route path="login" element={<LoginPage />} />
      <Route path="register" element={<RegisterPage />} />
      <Route path="politica-de-privacidad" element={<PrivacyPolicyPage />} />
      <Route path="terminos-de-servicio" element={<TermsOfServicePage />} />

      {/* Everything below here needs a session. */}
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route path="calendar" element={<CalendarPage />} />
          <Route path="chat" element={<ChatPage />} />
          <Route path="whatsapp" element={<WhatsAppPage />} />
          <Route path="reminders" element={<RemindersPage />} />
          <Route path="templates" element={<TemplatesPage />} />
        </Route>
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}

import { Navigate, Route, Routes } from 'react-router'

import { RequireAuth } from '@/components/require-auth'
import { Shell } from '@/components/shell'
import { AssistantPage } from '@/pages/assistant'
import { CalendarPage } from '@/pages/calendar'
import { HomePage } from '@/pages/home'
import { LoginPage } from '@/pages/login'
import { NotFoundPage } from '@/pages/not-found'
import { PrivacyPolicyPage } from '@/pages/privacy-policy'
import { RegisterPage } from '@/pages/register'
import { RemindersPage } from '@/pages/reminders'
import { SettingsPage } from '@/pages/settings'
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

      {/*
        The paths the five old tabs had. They are in browser histories and in
        bookmarks, and four of them still name a page, so they are redirects
        rather than four more 404s. /chat is deliberately not among them: the
        page is gone, and sending somebody somewhere else without saying so
        would look like the app had lost their conversations.
      */}
      <Route path="calendar" element={<Navigate to="/inicio" replace />} />
      <Route
        path="whatsapp"
        element={<Navigate to="/configuracion/whatsapp" replace />}
      />
      <Route
        path="reminders"
        element={<Navigate to="/configuracion/recordatorios" replace />}
      />
      <Route
        path="templates"
        element={<Navigate to="/configuracion/plantillas" replace />}
      />

      {/* Everything below here needs a session. */}
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route path="inicio" element={<CalendarPage />} />
          <Route path="configuracion" element={<SettingsPage />}>
            <Route index element={<AssistantPage />} />
            <Route path="whatsapp" element={<WhatsAppPage />} />
            <Route path="recordatorios" element={<RemindersPage />} />
            <Route path="plantillas" element={<TemplatesPage />} />
          </Route>
        </Route>
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}

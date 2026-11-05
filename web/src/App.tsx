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
import { SettingsPage } from '@/pages/settings'
import { TermsOfServicePage } from '@/pages/terms-of-service'

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

      {/* The assistant lived under Configuración before it got a tab. */}
      <Route
        path="configuracion/asistente"
        element={<Navigate to="/asistente" replace />}
      />

      {/* Everything below here needs a session. */}
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route path="inicio" element={<CalendarPage />} />
          <Route path="asistente" element={<AssistantPage />} />
          <Route path="configuracion" element={<SettingsPage />}>
            {/* One page; each path only says which row starts open. */}
            <Route index element={null} />
            <Route path="whatsapp" element={null} />
            <Route path="recordatorios" element={null} />
            <Route path="plantillas" element={null} />
          </Route>
        </Route>
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}

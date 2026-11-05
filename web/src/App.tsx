import { Navigate, Route, Routes } from 'react-router'

import { RequireAuth } from '@/components/require-auth'
import { Shell } from '@/components/shell'
import { AssistantPage } from '@/pages/assistant'
import { UnmatchedTaps } from '@/components/calendar/unmatched-taps'
import { CalendarPage } from '@/pages/calendar'
import { HomePage } from '@/pages/home'
import { IntegrationsPage } from '@/pages/integrations'
import { LoginPage } from '@/pages/login'
import { NotFoundPage } from '@/pages/not-found'
import { PrivacyPolicyPage } from '@/pages/privacy-policy'
import { RegisterPage } from '@/pages/register'
import { SettingsPage } from '@/pages/settings'
import { TermsOfServicePage } from '@/pages/terms-of-service'

/**
 * The day's work, with anything a patient said that still needs a human
 * above it.
 */
function AgendaPage() {
  return (
    <>
      <UnmatchedTaps />
      <CalendarPage />
    </>
  )
}

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
        Paths the app has had before. They are in browser histories and in
        bookmarks, so they are redirects rather than 404s. /chat is
        deliberately not among them: the page is gone, and sending somebody
        somewhere else without saying so would look like the app had lost
        their conversations.
      */}
      {[
        ['calendar', '/agenda'],
        ['inicio', '/agenda'],
        ['whatsapp', '/integraciones/whatsapp'],
        ['templates', '/integraciones/plantillas'],
        ['reminders', '/configuracion/recordatorios'],
        ['configuracion/whatsapp', '/integraciones/whatsapp'],
        ['configuracion/plantillas', '/integraciones/plantillas'],
        ['configuracion/asistente', '/asistente'],
      ].map(([from, to]) => (
        <Route key={from} path={from} element={<Navigate to={to} replace />} />
      ))}

      {/* Everything below here needs a session. */}
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route path="agenda" element={<AgendaPage />} />
          <Route path="asistente" element={<AssistantPage />} />
          {/* One page each; a sub-path only says which row starts open. */}
          <Route path="integraciones" element={<IntegrationsPage />}>
            <Route index element={null} />
            <Route path="whatsapp" element={null} />
            <Route path="plantillas" element={null} />
          </Route>
          <Route path="configuracion" element={<SettingsPage />}>
            <Route index element={null} />
            <Route path="negocio" element={null} />
            <Route path="recordatorios" element={null} />
            <Route path="apariencia" element={null} />
            <Route path="contrasena" element={null} />
          </Route>
        </Route>
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}

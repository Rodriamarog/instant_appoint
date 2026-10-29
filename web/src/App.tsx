import { Route, Routes } from 'react-router'

import { Shell } from '@/components/shell'
import { CalendarPage } from '@/pages/calendar'
import { HomePage } from '@/pages/home'
import { NotFoundPage } from '@/pages/not-found'

/**
 * The route table. Tickets #2-#8 replace the placeholder pages and add the
 * rest of the routes (login, register, chat, reminders, templates, whatsapp,
 * and the two legal pages).
 */
export function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<HomePage />} />
        <Route path="calendar" element={<CalendarPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}

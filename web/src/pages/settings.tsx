import { useEffect, useState, type ReactNode } from 'react'
import { useLocation } from 'react-router'
import {
  Bell,
  ChevronDown,
  FileText,
  MessageCircle,
  type LucideIcon,
} from 'lucide-react'

import { RemindersPage } from '@/pages/reminders'
import { TemplatesPage } from '@/pages/templates'
import { WhatsAppPage } from '@/pages/whatsapp'
import { cn } from '@/lib/utils'

/**
 * Everything a clinic sets up once and then leaves alone.
 *
 * One page of collapsible rows, grouped under small headings, rather than a
 * tab per setting: the whole list is readable at a glance, and a row only
 * loads what it needs once it is opened.
 *
 * Each row keeps the path it had as a tab, so /configuracion/plantillas and
 * friends — in bookmarks, and linked from inside the other rows — still land
 * on it, opened and scrolled into view.
 */
interface Row {
  id: string
  path: string
  icon: LucideIcon
  title: string
  subtitle: string
  content: () => ReactNode
}

const groups: { heading: string; rows: Row[] }[] = [
  {
    heading: 'WhatsApp',
    rows: [
      {
        id: 'whatsapp',
        path: '/configuracion/whatsapp',
        icon: MessageCircle,
        title: 'Número de WhatsApp',
        subtitle: 'El número desde el que salen recordatorios y respuestas',
        content: () => <WhatsAppPage />,
      },
      {
        id: 'recordatorios',
        path: '/configuracion/recordatorios',
        icon: Bell,
        title: 'Recordatorios',
        subtitle: 'Cuándo y cómo se avisa a tus pacientes antes de su cita',
        content: () => <RemindersPage />,
      },
      {
        id: 'plantillas',
        path: '/configuracion/plantillas',
        icon: FileText,
        title: 'Plantillas',
        subtitle: 'Los mensajes aprobados por Meta que puedes enviar',
        content: () => <TemplatesPage />,
      },
    ],
  },
]

const allRows = groups.flatMap((group) => group.rows)

export function SettingsPage() {
  const { pathname } = useLocation()
  const [open, setOpen] = useState<Set<string>>(() => new Set())

  // A path that names a row opens it — on arrival, and whenever a link
  // inside another row points at it — and brings it into view.
  useEffect(() => {
    const target = allRows.find((row) => pathname.replace(/\/$/, '') === row.path)
    if (!target) {
      return
    }

    setOpen((current) => new Set(current).add(target.id))
    document.getElementById(target.id)?.scrollIntoView?.({ block: 'start' })
  }, [pathname])

  const toggle = (id: string) =>
    setOpen((current) => {
      const next = new Set(current)
      if (!next.delete(id)) {
        next.add(id)
      }
      return next
    })

  return (
    <div className="mx-auto max-w-3xl space-y-8">
      <div>
        <h1 className="font-heading text-3xl font-bold tracking-[-0.03em]">
          Configuración
        </h1>
        <p className="mt-1.5 text-muted-foreground">
          Lo que decides una vez y después dejas en paz.
        </p>
      </div>

      {groups.map((group) => (
        <section key={group.heading} aria-labelledby={`grupo-${group.heading}`}>
          <h2
            id={`grupo-${group.heading}`}
            className="mb-3 text-sm font-semibold text-foreground"
          >
            {group.heading}
          </h2>
          <div className="overflow-hidden rounded-[14px] bg-background ring-1 ring-border">
            {group.rows.map((row) => (
              <SettingsRow
                key={row.id}
                row={row}
                open={open.has(row.id)}
                onToggle={() => toggle(row.id)}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function SettingsRow({
  row,
  open,
  onToggle,
}: {
  row: Row
  open: boolean
  onToggle: () => void
}) {
  const Icon = row.icon
  const panel = `${row.id}-panel`

  return (
    <div id={row.id} className="scroll-mt-24 border-b border-border last:border-b-0">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        aria-controls={panel}
        className="flex w-full cursor-pointer items-center gap-3 px-4 py-3.5 text-left transition-colors hover:bg-muted/60"
      >
        <span className="grid size-8 shrink-0 place-items-center rounded-[9px] bg-muted text-muted-foreground">
          <Icon className="size-4" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium text-foreground">{row.title}</span>
          <span className="block truncate text-xs text-muted-foreground">{row.subtitle}</span>
        </span>
        <ChevronDown
          className={cn(
            'size-4 shrink-0 text-muted-foreground transition-transform',
            open && 'rotate-180',
          )}
        />
      </button>
      {open && (
        <div id={panel} className="border-t border-border px-4 pb-5 pt-4">
          {row.content()}
        </div>
      )}
    </div>
  )
}

import {
  ArrowRight,
  ArrowUpRight,
  BellRing,
  Bot,
  CalendarCheck,
  Check,
  CheckCheck,
  Clock,
  Link2,
  MessageCircle,
  Plus,
  type LucideIcon,
} from 'lucide-react'
import { useMemo, useRef, type CSSProperties, type ReactNode } from 'react'
import { Link } from 'react-router'

import '@/components/landing/landing.css'
import { useReveal, useScrollSpy } from '@/components/landing/motion'
import { cn } from '@/lib/utils'

/**
 * Where every "talk to us" button goes.
 *
 * PLACEHOLDER NUMBER: replace 5210000000000 with the real sales WhatsApp
 * (country code + number, digits only) before this page is shown to anyone.
 */
const CONTACT_WHATSAPP = `https://wa.me/5210000000000?text=${encodeURIComponent(
  'Hola, me interesa InstantAppoint para mi negocio.',
)}`

const SECTIONS = [
  { id: 'por-que', label: 'Por qué' },
  { id: 'como-funciona', label: 'Cómo funciona' },
  { id: 'tu-agenda', label: 'Tu agenda' },
  { id: 'preguntas', label: 'Preguntas' },
]

/**
 * The public landing page, served at `/`.
 *
 * The whole pitch: you keep working exactly as you do now, and InstantAppoint
 * looks after the appointments and their reminders in the background. It is
 * sold as something that takes work away, never as one more tool to learn,
 * and the page is kept as quiet as the product is meant to be.
 *
 * Meta's app review fetches `/` signed out, so besides selling the product it
 * has to keep the two ways in (Iniciar sesión, Crear cuenta) and the two legal
 * pages reachable. Each of those four appears exactly once.
 */
export function HomePage() {
  const root = useRef<HTMLDivElement>(null)
  useReveal(root)

  return (
    <div ref={root} className="lp min-h-screen">
      <Nav />
      <main>
        <Hero />
        <NothingChanges />
        <HowItWorks />
        <YourDay />
        <Faq />
        <Closing />
      </main>
      <Footer />
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Pieces                                                              */
/* ------------------------------------------------------------------ */

function Logo() {
  return (
    <span className="lp-logo">
      Instant<b>Appoint</b>
    </span>
  )
}

function TalkButton({ children, small = false }: { children: ReactNode; small?: boolean }) {
  return (
    <a href={CONTACT_WHATSAPP} target="_blank" rel="noreferrer" className={cn('lp-btn', small && 'lp-btn--sm')}>
      {children}
      <ArrowUpRight />
    </a>
  )
}

/** A two-line headline, the second line in blue. */
function Headline({
  first,
  second,
  as: Tag = 'h2',
  className,
}: {
  first: string
  second: string
  as?: 'h1' | 'h2'
  className?: string
}) {
  return (
    <Tag data-reveal="lines" className={cn('lp-display', className)}>
      <span className="lp-line">
        <span>{first}</span>
      </span>
      <span className="lp-line">
        <span className="lp-accent">{second}</span>
      </span>
    </Tag>
  )
}

function SectionHead({ first, second, children }: { first: string; second: string; children: ReactNode }) {
  return (
    <div className="mb-12 max-w-3xl md:mb-14">
      <Headline first={first} second={second} />
      <p
        data-reveal
        style={{ '--d': '.15s' } as CSSProperties}
        className="mt-5 max-w-xl text-[18px] leading-relaxed text-[var(--muted)]"
      >
        {children}
      </p>
    </div>
  )
}

/** Stagger for items in a row. */
const delay = (i: number) => ({ '--d': `${i * 0.08}s` }) as CSSProperties

/* ------------------------------------------------------------------ */
/* Nav                                                                 */
/* ------------------------------------------------------------------ */

function Nav() {
  const progress = useRef<HTMLDivElement>(null)
  const ids = useMemo(() => SECTIONS.map((s) => s.id), [])
  const { scrolled, current } = useScrollSpy(ids, progress)

  return (
    <>
      <div ref={progress} className="lp-progress" aria-hidden />
      <header className={cn('lp-nav', scrolled && 'is-scrolled')}>
        <Link to="/" className="shrink-0">
          <Logo />
        </Link>

        <nav aria-label="Principal" className="lp-nav__links">
          {SECTIONS.map(({ id, label }) => (
            <a key={id} href={`#${id}`} aria-current={current === id ? 'true' : undefined}>
              {label}
            </a>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-4">
          <Link to="/login" className="text-sm font-medium whitespace-nowrap hover:underline">
            Iniciar sesión
          </Link>
          {/* The hero's button is a thumb's reach away on a phone, and the bar
              has no room for both. Iniciar sesión stays: it is the only one. */}
          <span className="hidden sm:inline-flex">
            <TalkButton small>Escríbenos</TalkButton>
          </span>
        </div>
      </header>
    </>
  )
}

/* ------------------------------------------------------------------ */
/* Hero                                                                */
/* ------------------------------------------------------------------ */

const BENEFITS: { icon: LucideIcon; title: string; body: string }[] = [
  { icon: Clock, title: 'Sin cambiar tu rutina', body: 'Tus clientes te siguen escribiendo al mismo WhatsApp.' },
  { icon: BellRing, title: 'Ningún recordatorio olvidado', body: 'Salen solos, antes de cada cita.' },
  { icon: CheckCheck, title: 'Sabes quién sí viene', body: 'Tus clientes confirman con un toque.' },
]

function Hero() {
  return (
    <section className="lp-hero" aria-label="InstantAppoint">
      <div className="lp-wrap">
        <div className="grid items-center gap-14 lg:grid-cols-[minmax(0,1.5fr)_minmax(0,.7fr)]">
          <div>
            <p data-reveal className="lp-eyebrow mb-6">
              <span className="size-1.5 rounded-full bg-current" />
              Citas y recordatorios por WhatsApp
            </p>

            <Headline
              as="h1"
              className="lp-display--hero"
              first="Trabaja como siempre."
              second="Tus citas se cuidan solas."
            />

            <p
              data-reveal
              style={{ '--d': '.25s' } as CSSProperties}
              className="mt-7 max-w-xl text-[19px] leading-[1.6] text-[var(--muted)]"
            >
              InstantAppoint agenda, confirma y recuerda cada cita por WhatsApp,
              en segundo plano. No cambias nada de cómo trabajas, y no hay nada
              nuevo que aprender.
            </p>

            <div
              data-reveal
              style={{ '--d': '.35s' } as CSSProperties}
              className="mt-9 flex flex-wrap items-center gap-x-8 gap-y-4"
            >
              <TalkButton>Escríbenos por WhatsApp</TalkButton>
              <a href="#como-funciona" className="lp-link-arrow">
                Ver cómo funciona <ArrowRight />
              </a>
            </div>
          </div>

          <div className="flex justify-center lg:justify-end">
            <PhoneChat />
          </div>
        </div>

        <ul className="mt-20 grid gap-8 border-t border-[var(--line)] pt-10 md:grid-cols-3">
          {BENEFITS.map((b, i) => (
            <li key={b.title} data-reveal style={delay(i)} className="flex gap-4">
              <span className="lp-icon">
                <b.icon />
              </span>
              <span>
                <span className="lp-title block text-[17px]">{b.title}</span>
                <span className="mt-1 block text-[15px] leading-relaxed text-[var(--muted)]">{b.body}</span>
              </span>
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}

const THREAD: { side: 'out' | 'in'; text: string; time: string; actions?: string[] }[] = [
  {
    side: 'out',
    text: 'Hola Ana 👋 Te recordamos tu cita en Clínica Sonrisa mañana jueves a las 10:00.',
    time: '09:00',
    actions: ['Confirmar', 'Cambiar hora'],
  },
  { side: 'in', text: 'Confirmar', time: '09:02' },
  { side: 'out', text: '¡Listo! Te esperamos mañana a las 10:00.', time: '09:02' },
  { side: 'in', text: 'Hola, ¿tienen lugar el sábado para mi hijo?', time: '11:40' },
  { side: 'out', text: '¡Claro! El sábado hay espacio a las 9:30 o a las 11:00. ¿Cuál te aparto?', time: '11:40' },
]

/** A conversation that plays itself once the phone scrolls into view. */
function PhoneChat() {
  return (
    <div data-reveal="phone" className="lp-phone" aria-hidden>
      <div className="lp-phone__screen">
        <div className="lp-phone__notch" />
        <div className="lp-phone__head">
          <div className="lp-phone__avatar">CS</div>
          <div className="min-w-0">
            <p className="text-sm font-semibold leading-tight">Clínica Sonrisa</p>
            <p className="text-[11px] text-[var(--muted)]">Cuenta de empresa</p>
          </div>
        </div>
        <div className="lp-phone__thread">
          {THREAD.map((m, i) => (
            <div
              key={i}
              className={cn('lp-msg', m.side === 'out' ? 'lp-msg--out' : 'lp-msg--in')}
              style={{ '--i': i } as CSSProperties}
            >
              {m.text}
              <span className="lp-msg__meta">{m.time}</span>
              {m.actions && (
                <span className="lp-msg__actions">
                  {m.actions.map((a) => (
                    <span key={a}>{a}</span>
                  ))}
                </span>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Nothing changes                                                     */
/* ------------------------------------------------------------------ */

const STAYS = ['Tu número de WhatsApp', 'La forma en que hablas con tus clientes', 'Tu manera de trabajar y tu horario']

const WE_DO = [
  'Anotamos cada cita que acuerdas con un cliente',
  'Mandamos el recordatorio a tiempo, sin que tengas que acordarte',
  'Te mostramos quién confirmó y quién no',
]

function NothingChanges() {
  return (
    <section id="por-que" className="scroll-mt-28 py-24 md:py-32">
      <div className="lp-wrap">
        <SectionHead first="Tú no cambias nada." second="Lo demás pasa solo.">
          InstantAppoint no es otra herramienta que aprender. Trabaja detrás de
          tu WhatsApp mientras tú atiendes.
        </SectionHead>

        <div className="grid gap-4 md:grid-cols-2">
          <div data-reveal style={delay(0)}>
            <div className="lp-card lp-card--soft">
              <h3 className="lp-title mb-6 text-[22px]">Lo que sigue igual</h3>
              <ul className="lp-list text-[var(--muted)]">
                {STAYS.map((s) => (
                  <li key={s}>
                    <Check />
                    {s}
                  </li>
                ))}
              </ul>
            </div>
          </div>
          <div data-reveal style={delay(1)}>
            <div className="lp-card">
              <h3 className="lp-title mb-6 text-[22px]">Lo que hacemos por ti</h3>
              <ul className="lp-list">
                {WE_DO.map((s) => (
                  <li key={s}>
                    <Check className="text-[var(--blue)]" />
                    {s}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

/* ------------------------------------------------------------------ */
/* How it works                                                        */
/* ------------------------------------------------------------------ */

const STEPS: { icon: LucideIcon; title: string; body: string }[] = [
  {
    icon: Link2,
    title: 'Conecta tu WhatsApp',
    body: 'Usamos el número que ya tienes, y te ayudamos a conectarlo.',
  },
  {
    icon: MessageCircle,
    title: 'Atiende como siempre',
    body: 'Tus clientes te escriben igual que hoy, y las citas que acuerdan quedan anotadas.',
  },
  {
    icon: BellRing,
    title: 'Nosotros recordamos',
    body: 'Antes de cada cita, tu cliente recibe su recordatorio y confirma con un toque.',
  },
]

function HowItWorks() {
  return (
    <section id="como-funciona" className="scroll-mt-28 bg-[var(--surface)] py-24 md:py-32">
      <div className="lp-wrap">
        <SectionHead first="Listo en tres pasos." second="Después, ni lo notas.">
          Lo configuramos contigo una sola vez. A partir de ahí, InstantAppoint
          trabaja solo.
        </SectionHead>

        <ol className="grid gap-4 md:grid-cols-3">
          {STEPS.map((s, i) => (
            <li key={s.title} data-reveal style={delay(i)}>
              <div className="lp-card">
                <div className="mb-8 flex items-center justify-between">
                  <span className="lp-icon">
                    <s.icon />
                  </span>
                  <span className="text-sm font-semibold text-[var(--faint)]">Paso {i + 1}</span>
                </div>
                <h3 className="lp-title text-[22px]">{s.title}</h3>
                <p className="mt-2 text-[15px] leading-relaxed text-[var(--muted)]">{s.body}</p>
              </div>
            </li>
          ))}
        </ol>
      </div>
    </section>
  )
}

/* ------------------------------------------------------------------ */
/* Your day                                                            */
/* ------------------------------------------------------------------ */

const DAY: { time: string; name: string; what: string; confirmed: boolean }[] = [
  { time: '09:00', name: 'Ana López', what: 'Limpieza', confirmed: true },
  { time: '10:00', name: 'Carlos Ruiz', what: 'Revisión', confirmed: true },
  { time: '11:30', name: 'María Fernández', what: 'Primera consulta', confirmed: false },
  { time: '12:30', name: 'Diego Torres', what: 'Ortodoncia', confirmed: true },
  { time: '16:00', name: 'Lucía Méndez', what: 'Limpieza', confirmed: false },
]

const POINTS: { icon: LucideIcon; title: string; body: string }[] = [
  { icon: CalendarCheck, title: 'Hoy, semana o mes', body: 'Tu agenda en la vista que prefieras.' },
  { icon: CheckCheck, title: 'Confirmaciones al momento', body: 'Cada respuesta de tus clientes se refleja sola.' },
  { icon: Bot, title: 'Un asistente discreto', body: 'Si quieres, responde y agenda por ti. Si no, solo toma nota.' },
]

function YourDay() {
  const confirmed = DAY.filter((d) => d.confirmed).length

  return (
    <section id="tu-agenda" className="scroll-mt-28 py-24 md:py-32">
      <div className="lp-wrap">
        <SectionHead first="Todo tu día," second="en un solo lugar.">
          Cuando quieras ver tu agenda, ahí está: quién viene, quién confirmó y
          quién falta.
        </SectionHead>

        <div className="grid items-start gap-10 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)] lg:gap-12">
          <div data-reveal className="lp-window">
            <div className="lp-window__bar">
              <i />
              <i />
              <i />
            </div>
            <div className="space-y-6 p-5 md:p-7">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h3 className="lp-title text-xl">Martes 17 de noviembre</h3>
                <span className="text-sm text-[var(--muted)]">
                  {confirmed} de {DAY.length} confirmadas
                </span>
              </div>
              <ul className="divide-y divide-[var(--line)] rounded-[10px] ring-1 ring-[var(--line)]">
                {DAY.map((row) => (
                  <li key={row.time} className="flex items-center gap-4 px-4 py-3">
                    <span className="w-12 shrink-0 text-sm font-medium tabular-nums text-[var(--muted)]">{row.time}</span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[15px] font-semibold">{row.name}</span>
                      <span className="block truncate text-xs text-[var(--muted)]">{row.what}</span>
                    </span>
                    <span className={cn('lp-status', row.confirmed ? 'lp-status--ok' : 'lp-status--wait')}>
                      {row.confirmed ? 'Confirmada' : 'Por confirmar'}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          </div>

          <ul className="grid gap-6">
            {POINTS.map((p, i) => (
              <li key={p.title} data-reveal style={delay(i + 1)} className="flex gap-4">
                <span className="lp-icon">
                  <p.icon />
                </span>
                <span>
                  <span className="lp-title block text-[18px]">{p.title}</span>
                  <span className="mt-1 block text-[15px] leading-relaxed text-[var(--muted)]">{p.body}</span>
                </span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  )
}

/* ------------------------------------------------------------------ */
/* FAQ, closing, footer                                                */
/* ------------------------------------------------------------------ */

const QUESTIONS: [string, string][] = [
  [
    '¿Tengo que aprender a usar algo nuevo?',
    'Casi nada. Tú sigues usando WhatsApp como siempre, e InstantAppoint trabaja por detrás. Cuando quieras ver tu día, abres tu agenda.',
  ],
  [
    '¿Tengo que cambiar de número?',
    'No. Conectas el número de WhatsApp Business que ya tienes, y puedes seguir usando la app en tu celular.',
  ],
  ['¿Mis clientes tienen que instalar algo?', 'No. Les llega un mensaje de WhatsApp normal y responden ahí mismo.'],
  ['¿Cómo empiezo?', 'Escríbenos por WhatsApp. Lo configuramos contigo y te enseñamos cómo se ve con tu propia agenda.'],
]

function Faq() {
  return (
    <section id="preguntas" className="scroll-mt-28 bg-[var(--surface)] py-24 md:py-32">
      <div className="lp-wrap grid gap-10 lg:grid-cols-[minmax(0,.8fr)_minmax(0,1.2fr)]">
        <Headline first="Preguntas" second="frecuentes." />
        <div className="lp-faq" data-reveal>
          {QUESTIONS.map(([q, a]) => (
            <details key={q}>
              <summary>
                {q}
                <span className="lp-faq__plus">
                  <Plus className="size-4" />
                </span>
              </summary>
              <p>{a}</p>
            </details>
          ))}
        </div>
      </div>
    </section>
  )
}

function Closing() {
  return (
    <section className="py-24 md:py-32">
      <div className="lp-wrap text-center">
        <Headline first="Sigue trabajando como siempre." second="Del resto nos encargamos." className="mx-auto max-w-3xl" />
        <p
          data-reveal
          style={{ '--d': '.15s' } as CSSProperties}
          className="mx-auto mt-6 max-w-lg text-[18px] leading-relaxed text-[var(--muted)]"
        >
          Escríbenos y te enseñamos InstantAppoint funcionando con tu agenda.
        </p>
        <div
          data-reveal
          style={{ '--d': '.25s' } as CSSProperties}
          className="mt-9 flex flex-wrap items-center justify-center gap-x-8 gap-y-4"
        >
          <TalkButton>Escríbenos por WhatsApp</TalkButton>
          <Link to="/register" className="lp-link-arrow">
            Crear cuenta <ArrowRight />
          </Link>
        </div>
      </div>
    </section>
  )
}

function Footer() {
  return (
    <footer className="border-t border-[var(--line)]">
      <div className="lp-wrap flex flex-col gap-6 py-10 text-sm text-[var(--muted)] md:flex-row md:items-center">
        <Logo />
        <span>© {new Date().getFullYear()} InstantAppoint</span>
        <nav aria-label="Legal y contacto" className="flex flex-wrap gap-x-6 gap-y-2 md:ml-auto">
          <Link to="/politica-de-privacidad" className="hover:text-[var(--ink)]">
            Política de privacidad
          </Link>
          <Link to="/terminos-de-servicio" className="hover:text-[var(--ink)]">
            Términos de servicio
          </Link>
          <a href={CONTACT_WHATSAPP} target="_blank" rel="noreferrer" className="hover:text-[var(--ink)]">
            WhatsApp
          </a>
        </nav>
      </div>
    </footer>
  )
}

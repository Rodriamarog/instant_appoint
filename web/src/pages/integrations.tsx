import { useCallback, useEffect, useState } from 'react'
import { Facebook, FileText, Instagram, Info, MessageCircle } from 'lucide-react'

import { SettingsGroup, SettingsRow, useOpenRows } from '@/components/settings/settings-row'
import { FacebookConnect } from '@/components/social/facebook-connect'
import { StatusPill } from '@/components/status-pill'
import { Button } from '@/components/ui/button'
import { errorMessage } from '@/lib/errors'
import {
  disconnectChannel,
  listSocialAccounts,
  type SocialAccount,
  type SocialChannel,
} from '@/lib/social-accounts'
import { TemplatesPage } from '@/pages/templates'
import { WhatsAppPage } from '@/pages/whatsapp'
import { listConnectedAccounts } from '@/lib/whatsapp-accounts'

/**
 * The channels patients write in on, and what each one needs set up.
 *
 * WhatsApp's templates sit beside its connection because a template belongs
 * to the WhatsApp Business account it was submitted from. Messenger and
 * Instagram share one Facebook login — Instagram's messages arrive through
 * the Page it is linked to — so either row's button connects both.
 */
const ROWS = ['whatsapp', 'plantillas', 'messenger', 'instagram']

type Social = Partial<Record<SocialChannel, SocialAccount>>

export function IntegrationsPage() {
  const { isOpen, toggle } = useOpenRows('/integraciones', ROWS)
  // null while unknown, so no pill claims a state before it is read.
  const [connected, setConnected] = useState<boolean | null>(null)

  useEffect(() => {
    listConnectedAccounts()
      .then((accounts) => setConnected(accounts.length > 0))
      .catch(() => setConnected(null))
  }, [])

  // undefined while unknown, for the same reason.
  const [social, setSocial] = useState<Social | undefined>(undefined)
  const reloadSocial = useCallback(() => {
    listSocialAccounts()
      .then(setSocial)
      .catch(() => setSocial(undefined))
  }, [])
  useEffect(reloadSocial, [reloadSocial])

  return (
    <div className="mx-auto max-w-3xl space-y-8">
      <div>
        <h1 className="font-heading text-3xl font-bold tracking-[-0.03em]">Integraciones</h1>
        <p className="mt-1.5 text-muted-foreground">
          Los canales por los que te escriben tus pacientes.
        </p>
      </div>

      <SettingsGroup heading="WhatsApp">
        <SettingsRow
          id="whatsapp"
          icon={MessageCircle}
          title="Número de WhatsApp"
          subtitle="El número desde el que salen recordatorios y respuestas"
          trailing={
            connected === null ? undefined : (
              <StatusPill tone={connected ? 'settled' : 'waiting'}>
                {connected ? 'Conectado' : 'Sin conectar'}
              </StatusPill>
            )
          }
          open={isOpen('whatsapp')}
          onToggle={() => toggle('whatsapp')}
        >
          <WhatsAppPage />
        </SettingsRow>
        <SettingsRow
          id="plantillas"
          icon={FileText}
          title="Plantillas"
          subtitle="Los mensajes aprobados por Meta que puedes enviar"
          open={isOpen('plantillas')}
          onToggle={() => toggle('plantillas')}
        >
          <TemplatesPage />
        </SettingsRow>
      </SettingsGroup>

      <SettingsGroup heading="Facebook e Instagram">
        <SettingsRow
          id="messenger"
          icon={Facebook}
          title="Facebook Messenger"
          subtitle={
            social?.messenger
              ? `Página: ${social.messenger.name}`
              : 'Contesta y agenda desde los mensajes de tu página de Facebook'
          }
          trailing={social && <ConnectedPill on={!!social.messenger} />}
          open={isOpen('messenger')}
          onToggle={() => toggle('messenger')}
        >
          <MessengerPanel social={social} onChange={reloadSocial} />
        </SettingsRow>
        <SettingsRow
          id="instagram"
          icon={Instagram}
          title="Instagram"
          subtitle={
            social?.instagram
              ? `@${social.instagram.username || social.instagram.name}`
              : 'Contesta y agenda desde los mensajes directos de Instagram'
          }
          trailing={social && <ConnectedPill on={!!social.instagram} />}
          open={isOpen('instagram')}
          onToggle={() => toggle('instagram')}
        >
          <InstagramPanel social={social} onChange={reloadSocial} />
        </SettingsRow>
      </SettingsGroup>
    </div>
  )
}

function ConnectedPill({ on }: { on: boolean }) {
  return (
    <StatusPill tone={on ? 'settled' : 'waiting'}>{on ? 'Conectado' : 'Sin conectar'}</StatusPill>
  )
}

/** The note both rows carry: reminders still need a WhatsApp number. */
function RemindersNote() {
  return (
    <p className="flex gap-2 text-xs text-muted-foreground">
      <Info className="mt-px size-3.5 shrink-0" />
      <span>
        El asistente contesta y agenda igual que en WhatsApp. Los recordatorios siguen saliendo por
        WhatsApp, así que el asistente le pide su número al paciente.
      </span>
    </p>
  )
}

function Disconnect({ channel, onDone }: { channel: SocialChannel; onDone: () => void }) {
  const [busy, setBusy] = useState(false)
  const [failure, setFailure] = useState('')

  return (
    <div className="space-y-1">
      <Button
        variant="outline"
        size="sm"
        disabled={busy}
        onClick={() => {
          setBusy(true)
          setFailure('')
          disconnectChannel(channel)
            .then(onDone)
            .catch((err) => setFailure(errorMessage(err, 'No se pudo desconectar.')))
            .finally(() => setBusy(false))
        }}
      >
        {busy ? 'Desconectando...' : 'Desconectar'}
      </Button>
      {failure && <p className="text-sm text-destructive">{failure}</p>}
    </div>
  )
}

function MessengerPanel({
  social,
  onChange,
}: {
  social: Social | undefined
  onChange: () => void
}) {
  const page = social?.messenger

  return (
    <div className="space-y-4">
      {page ? (
        <>
          <p className="text-sm">
            Conectado a la página <span className="font-semibold">{page.name}</span>. Los mensajes
            que le escriban llegan al asistente.
          </p>
          <div className="flex flex-wrap items-start gap-2">
            <Disconnect channel="messenger" onDone={onChange} />
            <FacebookConnect label="Cambiar de página" onConnected={onChange} />
          </div>
        </>
      ) : (
        <>
          <p className="text-sm text-muted-foreground">
            Inicia sesión con la cuenta de Facebook que administra la página de tu negocio y elige
            esa página. Si tiene una cuenta de Instagram profesional vinculada, se conecta también.
          </p>
          <FacebookConnect onConnected={onChange} />
        </>
      )}
      <RemindersNote />
    </div>
  )
}

function InstagramPanel({
  social,
  onChange,
}: {
  social: Social | undefined
  onChange: () => void
}) {
  const account = social?.instagram

  if (account) {
    return (
      <div className="space-y-4">
        <p className="text-sm">
          Conectado a <span className="font-semibold">@{account.username || account.name}</span>, a
          través de la página de Facebook
          {social?.messenger ? ` ${social.messenger.name}` : ''}.
        </p>
        <p className="text-xs text-muted-foreground">
          Si los mensajes no llegan, revisa en Instagram: Configuración › Mensajes y respuestas a
          historias › Herramientas conectadas › Permitir acceso a mensajes.
        </p>
        <Disconnect channel="instagram" onDone={onChange} />
        <RemindersNote />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {social?.messenger && (
        <p className="rounded-[10px] bg-muted px-3 py-2 text-sm">
          La página <span className="font-semibold">{social.messenger.name}</span> no tiene una
          cuenta de Instagram profesional vinculada.
        </p>
      )}
      <ol className="list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground">
        <li>Tu cuenta de Instagram debe ser profesional (de empresa o de creador).</li>
        <li>
          Vincúlala a la página de Facebook de tu negocio, desde la configuración de la página.
        </li>
        <li>
          En Instagram, activa Configuración › Mensajes y respuestas a historias › Herramientas
          conectadas › Permitir acceso a mensajes.
        </li>
        <li>Conecta con Facebook y elige esa página.</li>
      </ol>
      <FacebookConnect onConnected={onChange} />
      <RemindersNote />
    </div>
  )
}

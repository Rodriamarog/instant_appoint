import { useEffect, useState } from 'react'
import { Facebook, FileText, Instagram, MessageCircle } from 'lucide-react'

import { SettingsGroup, SettingsRow, useOpenRows } from '@/components/settings/settings-row'
import { StatusPill } from '@/components/status-pill'
import { TemplatesPage } from '@/pages/templates'
import { WhatsAppPage } from '@/pages/whatsapp'
import { listConnectedAccounts } from '@/lib/whatsapp-accounts'

/**
 * The channels patients write in on, and what each one needs set up.
 *
 * WhatsApp is the only one live today; its templates sit beside its
 * connection because a template belongs to the WhatsApp Business account it
 * was submitted from. Messenger and Instagram are listed as coming, so the
 * page reads as the place they will appear rather than as WhatsApp settings.
 */
const ROWS = ['whatsapp', 'plantillas']

export function IntegrationsPage() {
  const { isOpen, toggle } = useOpenRows('/integraciones', ROWS)
  // null while unknown, so no pill claims a state before it is read.
  const [connected, setConnected] = useState<boolean | null>(null)

  useEffect(() => {
    listConnectedAccounts()
      .then((accounts) => setConnected(accounts.length > 0))
      .catch(() => setConnected(null))
  }, [])

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

      <SettingsGroup heading="Próximamente">
        <SettingsRow
          id="messenger"
          icon={Facebook}
          title="Facebook Messenger"
          subtitle="Contesta y agenda desde los mensajes de tu página de Facebook"
          trailing={<StatusPill tone="off">Próximamente</StatusPill>}
          open={false}
          onToggle={() => {}}
          disabled
        />
        <SettingsRow
          id="instagram"
          icon={Instagram}
          title="Instagram"
          subtitle="Contesta y agenda desde los mensajes directos de Instagram"
          trailing={<StatusPill tone="off">Próximamente</StatusPill>}
          open={false}
          onToggle={() => {}}
          disabled
        />
      </SettingsGroup>
    </div>
  )
}

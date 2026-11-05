import { useEffect, useState } from 'react'
import { Bell, Building2, KeyRound, Palette } from 'lucide-react'

import { ReminderSettingsForm } from '@/components/reminders/settings-form'
import { BusinessForm } from '@/components/settings/business-form'
import { PasswordForm } from '@/components/settings/password-form'
import { SettingsGroup, SettingsRow, useOpenRows } from '@/components/settings/settings-row'
import { ThemeField } from '@/components/settings/theme-field'
import { StatusPill } from '@/components/status-pill'
import { updateBusinessName, useCurrentUser } from '@/lib/auth'
import {
  DEFAULT_SETTINGS,
  TIMING_OPTIONS,
  loadBusinessSettings,
  saveBusinessSettings,
  type BusinessSettings,
} from '@/lib/business-settings'
import { useTheme } from '@/lib/theme'

/**
 * Everything a clinic sets up once and then leaves alone: the business, its
 * reminders, how the app looks, and the account.
 *
 * The settings row is loaded once here and handed to every form that writes
 * to it, so two forms on the page can never save over each other with stale
 * copies of the same row.
 */
const ROWS = ['negocio', 'recordatorios', 'apariencia', 'contrasena']

const THEME_LABEL = { light: 'Claro', dark: 'Oscuro', system: 'Sistema' }

export function SettingsPage() {
  const { isOpen, toggle } = useOpenRows('/configuracion', ROWS)
  const user = useCurrentUser()
  const { preference } = useTheme()
  const [settings, setSettings] = useState<BusinessSettings | null>(null)
  const [loadError, setLoadError] = useState('')

  useEffect(() => {
    loadBusinessSettings()
      .then(setSettings)
      .catch(() => {
        // Shown with the defaults, but said: saving from here would write
        // over a row that may well exist.
        setSettings(DEFAULT_SETTINGS)
        setLoadError('No se pudo cargar tu configuración. Lo que guardes podría reemplazarla.')
      })
  }, [])

  const save = async (patch: Partial<BusinessSettings>) => {
    if (!settings) {
      return
    }
    setSettings(await saveBusinessSettings({ ...settings, ...patch }))
    setLoadError('')
  }

  const lead = settings && TIMING_OPTIONS.find((o) => o.minutes === settings.timing_minutes)

  return (
    <div className="mx-auto max-w-3xl space-y-8">
      <div>
        <h1 className="font-heading text-3xl font-bold tracking-[-0.03em]">Configuración</h1>
        <p className="mt-1.5 text-muted-foreground">
          Lo que decides una vez y después dejas en paz.
        </p>
      </div>

      {loadError && <p className="text-sm text-destructive">{loadError}</p>}

      <SettingsGroup heading="Negocio">
        <SettingsRow
          id="negocio"
          icon={Building2}
          title="Datos del negocio"
          subtitle={
            [user?.business_name || 'Sin nombre', settings?.timezone.replace('_', ' ')]
              .filter(Boolean)
              .join(' · ')
          }
          open={isOpen('negocio')}
          onToggle={() => toggle('negocio')}
        >
          {settings && (
            <BusinessForm
              businessName={user?.business_name ?? ''}
              settings={settings}
              onSaveName={updateBusinessName}
              onSaveSettings={save}
            />
          )}
        </SettingsRow>
        <SettingsRow
          id="recordatorios"
          icon={Bell}
          title="Recordatorios"
          subtitle={
            !settings
              ? 'Cuándo y con qué plantilla se avisa a tus pacientes'
              : settings.is_active
                ? `${lead ? `${lead.label} antes de cada cita` : 'Antes de cada cita'}${settings.template_name ? '' : ' · falta elegir plantilla'}`
                : 'Apagados'
          }
          trailing={
            settings && (
              <StatusPill tone={settings.is_active ? 'settled' : 'off'}>
                {settings.is_active ? 'Encendidos' : 'Apagados'}
              </StatusPill>
            )
          }
          open={isOpen('recordatorios')}
          onToggle={() => toggle('recordatorios')}
        >
          {settings && <ReminderSettingsForm settings={settings} onSave={save} />}
        </SettingsRow>
      </SettingsGroup>

      <SettingsGroup heading="Apariencia">
        <SettingsRow
          id="apariencia"
          icon={Palette}
          title="Tema"
          subtitle={THEME_LABEL[preference]}
          open={isOpen('apariencia')}
          onToggle={() => toggle('apariencia')}
        >
          <ThemeField />
        </SettingsRow>
      </SettingsGroup>

      <SettingsGroup heading="Cuenta">
        <SettingsRow
          id="contrasena"
          icon={KeyRound}
          title="Contraseña"
          subtitle={user ? `Sesión iniciada como ${user.email}` : 'Cambia tu contraseña'}
          open={isOpen('contrasena')}
          onToggle={() => toggle('contrasena')}
        >
          <PasswordForm />
        </SettingsRow>
      </SettingsGroup>
    </div>
  )
}

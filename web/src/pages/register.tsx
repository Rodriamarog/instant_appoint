import { ArrowRight } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'

import { AuthField, AuthLayout } from '@/components/auth-layout'
import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { register } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'

export function RegisterPage() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [name, setName] = useState('')
  const [businessName, setBusinessName] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  // Registration deliberately does not navigate: the account still has to be
  // verified before it can sign in.

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsLoading(true)
    setError('')
    setSuccess('')

    if (password !== confirmPassword) {
      setError('Las contraseñas no coinciden')
      setIsLoading(false)
      return
    }

    try {
      const { verificationSent } = await register({
        email,
        password,
        name,
        businessName,
      })

      setSuccess(
        verificationSent
          ? '¡Cuenta creada! Revisa tu correo y abre el enlace de verificación para activarla.'
          : 'La cuenta se creó, pero no se pudo enviar el correo de verificación. Pide a un administrador que la verifique.',
      )

      // Clear form
      setEmail('')
      setPassword('')
      setConfirmPassword('')
      setName('')
      setBusinessName('')
    } catch (err) {
      setError(errorMessage(err, 'No se pudo crear la cuenta'))
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <AuthLayout
      title="Crea tu cuenta"
      subtitle="Toma un minuto. Después conectas tu WhatsApp y nosotros nos encargamos de los recordatorios."
    >
      <form onSubmit={handleRegister} className="space-y-5">
        {error && <Alert variant="destructive">{error}</Alert>}
        {success && (
          <Alert variant="success" role="status">
            {success}
          </Alert>
        )}

        <AuthField>
          <Label htmlFor="name">Tu nombre completo</Label>
          <Input
            id="name"
            name="name"
            type="text"
            autoComplete="name"
            placeholder="p. ej. Juan Pérez"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </AuthField>
        <AuthField>
          <Label htmlFor="businessName">Nombre del negocio</Label>
          <Input
            id="businessName"
            name="businessName"
            type="text"
            autoComplete="organization"
            placeholder="p. ej. Clínica Dental Pérez"
            value={businessName}
            onChange={(e) => setBusinessName(e.target.value)}
          />
        </AuthField>
        <AuthField>
          <Label htmlFor="email">Correo electrónico</Label>
          <Input
            id="email"
            name="email"
            type="email"
            autoComplete="email"
            placeholder="tu@correo.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </AuthField>

        <div className="grid gap-5 sm:grid-cols-2">
          <AuthField>
            <Label htmlFor="password">Contraseña</Label>
            <Input
              id="password"
              name="password"
              type="password"
              autoComplete="new-password"
              placeholder="Mínimo 8 caracteres"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              minLength={8}
            />
          </AuthField>
          <AuthField>
            <Label htmlFor="confirmPassword">Confirmar contraseña</Label>
            <Input
              id="confirmPassword"
              name="confirmPassword"
              type="password"
              autoComplete="new-password"
              placeholder="Repítela"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              minLength={8}
            />
          </AuthField>
        </div>

        <Button type="submit" size="lg" className="w-full" disabled={isLoading}>
          {isLoading ? 'Creando cuenta...' : 'Crear cuenta'}
          {!isLoading && <ArrowRight data-icon="inline-end" />}
        </Button>
      </form>

      <p className="mt-8 border-t pt-6 text-sm text-muted-foreground">
        ¿Ya tienes cuenta?{' '}
        <Link to="/login" className="font-semibold text-brand hover:underline">
          Inicia sesión
        </Link>
      </p>
    </AuthLayout>
  )
}

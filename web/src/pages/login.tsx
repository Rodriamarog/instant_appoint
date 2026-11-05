import { ArrowRight } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate } from 'react-router'

import { AuthField, AuthLayout } from '@/components/auth-layout'
import { Alert } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { requestVerification, signIn } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'

export function LoginPage() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [isResendingVerification, setIsResendingVerification] = useState(false)
  const [error, setError] = useState('')
  const [verificationMessage, setVerificationMessage] = useState('')
  const navigate = useNavigate()

  const handleResendVerification = async () => {
    if (!email) {
      setError('Escribe primero tu correo electrónico')
      return
    }

    setIsResendingVerification(true)
    setError('')
    setVerificationMessage('')

    try {
      await requestVerification(email)
      setVerificationMessage(
        'Te enviamos el correo de verificación. Revisa tu bandeja de entrada.',
      )
    } catch (err) {
      setError(errorMessage(err, 'No se pudo enviar el correo de verificación'))
    } finally {
      setIsResendingVerification(false)
    }
  }

  const handleLogin = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setIsLoading(true)
    setError('')
    setVerificationMessage('')

    const formData = new FormData(e.currentTarget)
    const emailVal = (formData.get('email') as string) || email
    const passwordVal = (formData.get('password') as string) || password

    try {
      // An unverified account is refused by lib/auth, which clears the
      // session again rather than leaving a half-signed-in page.
      await signIn(emailVal, passwordVal)
      navigate('/inicio')
    } catch (err) {
      setError(errorMessage(err, 'No se pudo iniciar sesión'))
      setIsLoading(false) // Only reset on error; success navigates away.
    }
  }

  return (
    <AuthLayout
      title="Inicia sesión"
      subtitle="Entra para ver tu agenda del día y quién ya confirmó."
    >
      <form onSubmit={handleLogin} className="space-y-5">
        {error && <Alert variant="destructive">{error}</Alert>}
        {verificationMessage && (
          <Alert variant="success" role="status">
            {verificationMessage}
          </Alert>
        )}

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

        <AuthField>
          <Label htmlFor="password">Contraseña</Label>
          <Input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            placeholder="Escribe tu contraseña"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
          />
        </AuthField>

        <Button type="submit" size="lg" className="w-full" disabled={isLoading}>
          {isLoading ? 'Iniciando sesión...' : 'Iniciar sesión'}
          {!isLoading && <ArrowRight data-icon="inline-end" />}
        </Button>
      </form>

      <div className="mt-8 space-y-3 border-t pt-6 text-sm text-muted-foreground">
        <p>
          ¿No tienes cuenta?{' '}
          <Link to="/register" className="font-semibold text-brand hover:underline">
            Crear cuenta
          </Link>
        </p>
        <p>
          ¿Aún no verificas tu correo?{' '}
          <button
            type="button"
            onClick={handleResendVerification}
            disabled={isResendingVerification}
            className="cursor-pointer font-semibold text-brand hover:underline disabled:opacity-50"
          >
            {isResendingVerification ? 'Enviando...' : 'Reenviar verificación'}
          </button>
        </p>
      </div>
    </AuthLayout>
  )
}

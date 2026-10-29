import { useState } from 'react'
import { Link, useNavigate } from 'react-router'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
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
      setError('Please enter your email address first')
      return
    }

    setIsResendingVerification(true)
    setError('')
    setVerificationMessage('')

    try {
      await requestVerification(email)
      setVerificationMessage('Verification email sent! Check your inbox.')
    } catch (err) {
      setError(errorMessage(err, 'Failed to send verification email'))
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
      navigate('/calendar')
    } catch (err) {
      setError(errorMessage(err, 'Failed to login'))
      setIsLoading(false) // Only reset on error; success navigates away.
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <Card className="w-full max-w-md">
        <CardHeader className="space-y-1">
          <CardTitle className="text-2xl font-bold text-center">
            InstantAppoint
          </CardTitle>
          <CardDescription className="text-center">
            Sign in to your account to manage appointments
          </CardDescription>
        </CardHeader>
        <form onSubmit={handleLogin}>
          <CardContent className="space-y-4">
            {error && (
              <div
                role="alert"
                className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded"
              >
                {error}
              </div>
            )}
            {verificationMessage && (
              <div
                role="status"
                className="bg-green-50 border border-green-200 text-green-700 px-4 py-3 rounded"
              >
                {verificationMessage}
              </div>
            )}

            <div className="space-y-2">
              <label htmlFor="email" className="text-sm font-medium">
                Email
              </label>
              <Input
                id="email"
                name="email"
                type="email"
                placeholder="Enter your email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>

            <div className="space-y-2">
              <label htmlFor="password" className="text-sm font-medium">
                Password
              </label>
              <Input
                id="password"
                name="password"
                type="password"
                placeholder="Enter your password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </div>
          </CardContent>

          <CardFooter className="flex flex-col space-y-4">
            <Button type="submit" className="w-full" disabled={isLoading}>
              {isLoading ? 'Signing in...' : 'Sign In'}
            </Button>

            <div className="text-sm text-center text-gray-600 space-y-2">
              <p>
                Don&apos;t have an account?{' '}
                <Link to="/register" className="text-blue-600 hover:underline">
                  Sign up
                </Link>
              </p>
              <p>
                Haven&apos;t verified your email?{' '}
                <button
                  type="button"
                  onClick={handleResendVerification}
                  disabled={isResendingVerification}
                  className="text-blue-600 hover:underline disabled:opacity-50"
                >
                  {isResendingVerification ? 'Sending...' : 'Resend verification'}
                </button>
              </p>
            </div>
          </CardFooter>
        </form>
      </Card>
    </div>
  )
}

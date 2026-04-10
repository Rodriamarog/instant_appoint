'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { pb } from '@/lib/pocketbase'

export default function LoginPage() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [isResendingVerification, setIsResendingVerification] = useState(false)
  const [error, setError] = useState('')
  const [verificationMessage, setVerificationMessage] = useState('')
  const router = useRouter()

  const handleResendVerification = async () => {
    if (!email) {
      setError('Please enter your email address first')
      return
    }

    setIsResendingVerification(true)
    setError('')
    setVerificationMessage('')

    try {
      await pb.collection('users').requestVerification(email)
      setVerificationMessage('Verification email sent! Check your inbox.')
    } catch (err: any) {
      setError(err.message || 'Failed to send verification email')
    } finally {
      setIsResendingVerification(false)
    }
  }

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsLoading(true)
    setError('')
    setVerificationMessage('')

    try {
      console.log('Attempting to login with:', email)
      await pb.collection('users').authWithPassword(email, password)

      // Check if user is verified
      const user = pb.authStore.model
      console.log('User logged in:', user)

      if (user && !user.verified) {
        pb.authStore.clear()
        setError('Please verify your email address before logging in. Check your email for the verification link.')
        return
      }

      console.log('Redirecting to dashboard...')

      // Don't reset loading state - we're redirecting
      window.location.href = '/calendar'
      return // Exit early to prevent setIsLoading(false)
    } catch (err: any) {
      console.error('Login error:', err)
      setError(err.message || 'Failed to login')
      setIsLoading(false) // Only reset on error
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <Card className="w-full max-w-md">
        <CardHeader className="space-y-1">
          <CardTitle className="text-2xl font-bold text-center">InstantAppoint</CardTitle>
          <CardDescription className="text-center">
            Sign in to your account to manage appointments
          </CardDescription>
        </CardHeader>
        <form onSubmit={handleLogin}>
          <CardContent className="space-y-4">
            {error && (
              <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded">
                {error}
              </div>
            )}
            {verificationMessage && (
              <div className="bg-green-50 border border-green-200 text-green-700 px-4 py-3 rounded">
                {verificationMessage}
              </div>
            )}

            <div className="space-y-2">
              <label htmlFor="email" className="text-sm font-medium">
                Email
              </label>
              <Input
                id="email"
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
                type="password"
                placeholder="Enter your password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </div>
          </CardContent>

          <CardFooter className="flex flex-col space-y-4">
            <Button
              type="submit"
              className="w-full"
              disabled={isLoading}
            >
              {isLoading ? 'Signing in...' : 'Sign In'}
            </Button>

            <div className="text-sm text-center text-gray-600 space-y-2">
              <p>
                Don't have an account?{' '}
                <a href="/register" className="text-blue-600 hover:underline">
                  Sign up
                </a>
              </p>
              <p>
                Haven't verified your email?{' '}
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
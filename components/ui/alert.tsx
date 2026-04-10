import * as React from 'react'

export function Alert({ variant = 'default', children, className = '' }: { variant?: 'default' | 'destructive'; children: React.ReactNode; className?: string }) {
  const base = 'rounded-md border px-4 py-3 text-sm'
  const variants = {
    default: 'bg-gray-50 border-gray-200 text-gray-800',
    destructive: 'bg-red-50 border-red-200 text-red-800',
  }
  return <div className={`${base} ${variants[variant]} ${className}`}>{children}</div>
}

export function AlertDescription({ children }: { children: React.ReactNode }) {
  return <p>{children}</p>
}

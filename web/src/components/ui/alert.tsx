import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'

const alertVariants = cva(
  'relative w-full rounded-3xl px-4 py-3 text-sm ring-1 ring-inset',
  {
    variants: {
      variant: {
        default: 'bg-card text-card-foreground ring-foreground/10',
        destructive: 'bg-destructive/10 text-destructive ring-destructive/20',
      },
    },
    defaultVariants: { variant: 'default' },
  },
)

function Alert({
  className,
  variant,
  ...props
}: React.ComponentProps<'div'> & VariantProps<typeof alertVariants>) {
  return (
    <div
      data-slot="alert"
      role="alert"
      className={cn(alertVariants({ variant }), className)}
      {...props}
    />
  )
}

function AlertDescription({ className, ...props }: React.ComponentProps<'p'>) {
  return <p data-slot="alert-description" className={cn('text-sm', className)} {...props} />
}

export { Alert, AlertDescription, alertVariants }

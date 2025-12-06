import * as React from 'react'

import { cn } from '@/lib/utils'

function Input({ className, type, ...props }: React.ComponentProps<'input'>) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        // Base: Clean, understated
        'h-9 w-full min-w-0 rounded-md border border-border bg-card px-3 py-1 text-base text-foreground',
        // Placeholder: Soft but readable
        'placeholder:text-muted-foreground/70',
        // Selection: Accent highlight
        'selection:bg-accent/20 selection:text-foreground',
        // Transition: Smooth focus
        'transition-all duration-200 ease-out outline-none',
        // Focus: Subtle border change, no harsh ring
        'focus-visible:border-foreground/30 focus-visible:bg-background',
        // File inputs
        'file:text-foreground file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium',
        // Disabled
        'disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
        // Dark mode
        'dark:bg-card dark:border-border dark:focus-visible:border-foreground/30',
        // Invalid state
        'aria-invalid:border-destructive aria-invalid:focus-visible:border-destructive',
        'md:text-sm',
        className,
      )}
      {...props}
    />
  )
}

export { Input }

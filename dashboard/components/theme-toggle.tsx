'use client'

import * as React from 'react'
import { Moon, Sun } from 'lucide-react'
import { useTheme } from 'next-themes'

export function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const [mounted, setMounted] = React.useState(false)

  // Avoid hydration mismatch
  React.useEffect(() => {
    setMounted(true)
  }, [])

  if (!mounted) {
    return (
      <button
        className="fixed-top-right flex h-10 w-10 items-center justify-center rounded-full border border-border/50 bg-card/80 backdrop-blur-sm"
        aria-label="Toggle theme"
      >
        <Sun className="h-4 w-4 text-muted-foreground" />
      </button>
    )
  }

  return (
    <button
      onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
      className="fixed-top-right flex h-10 w-10 items-center justify-center rounded-full border border-border/50 bg-card/80 backdrop-blur-sm transition-all duration-200 hover:bg-card hover:border-foreground/20 hover:scale-105 active:scale-95 group"
      aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`}
    >
      {theme === 'dark' ? (
        <Sun className="h-4 w-4 text-muted-foreground transition-colors duration-200 group-hover:text-foreground" />
      ) : (
        <Moon className="h-4 w-4 text-muted-foreground transition-colors duration-200 group-hover:text-foreground" />
      )}
    </button>
  )
}


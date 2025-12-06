'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useToast } from '@/hooks/use-toast'

export function Waitlist() {
  const [email, setEmail] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const { toast } = useToast()

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    
    if (!email || !email.includes('@')) {
      toast({
        title: 'Invalid email',
        description: 'Please enter a valid email address.',
        variant: 'destructive',
      })
      return
    }

    setIsLoading(true)

    try {
      const response = await fetch('/api/waitlist', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ email }),
      })

      const data = await response.json()

      if (!response.ok) {
        // Handle rate limiting specifically
        if (response.status === 429) {
          const retryAfter = data.retryAfter || 60
          throw new Error(
            `Too many requests. Please try again in ${Math.ceil(retryAfter / 60)} minute${retryAfter > 60 ? 's' : ''}.`
          )
        }
        throw new Error(data.error || 'Failed to join waitlist')
      }

      toast({
        title: 'Success!',
        description: 'You\'ve been added to the waitlist. We\'ll be in touch soon!',
      })
      setEmail('')
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Something went wrong. Please try again.',
        variant: 'destructive',
      })
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col sm:flex-row gap-3 max-w-md">
      <Input
        type="email"
        placeholder="Enter your email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        disabled={isLoading}
        required
        className="flex-1"
      />
      <Button type="submit" disabled={isLoading} className="whitespace-nowrap">
        {isLoading ? 'Adding you..' : 'Join Waitlist'}
      </Button>
    </form>
  )
}


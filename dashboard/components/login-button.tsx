"use client"

import { Button } from "@/components/ui/button"
import { getLoginUrl } from "@/lib/auth"

export function LoginButton() {
  const handleLogin = () => {
    window.location.href = getLoginUrl()
  }

  return (
    <Button 
      onClick={handleLogin}
      variant="outline"
      className="font-serif text-sm border-none shadow-none underline"
    >
      Sign in
    </Button>
  )
}


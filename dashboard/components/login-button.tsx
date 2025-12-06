"use client"

import { Button } from "@/components/ui/button"
import { signIn } from "@/lib/auth-client"

export function LoginButton() {
  const handleLogin = async () => {
    try {
      await signIn.social({
        provider: "gitlab",
        callbackURL: "/dashboard",
      })
    } catch (error) {
      console.error("Login failed:", error)
    }
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


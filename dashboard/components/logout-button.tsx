"use client"

import { Button } from "@/components/ui/button"
import { signOut } from "@/lib/auth-client"
import { useRouter } from "next/navigation"

export function LogoutButton() {
  const router = useRouter()

  const handleLogout = async () => {
    try {
      await signOut()
      router.push("/")
    } catch (error) {
      console.error("Logout failed:", error)
    }
  }

  return (
    <Button
      onClick={handleLogout}
      variant="outline"
      className="font-serif text-sm border-none shadow-none underline"
    >
      Sign out
    </Button>
  )
}


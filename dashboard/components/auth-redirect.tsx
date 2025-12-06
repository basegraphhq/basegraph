"use client"

import { useSession } from "@/lib/auth-client"
import { useRouter } from "next/navigation"
import { useEffect } from "react"

/**
 * Client component that redirects authenticated users to the dashboard.
 * Place this in pages where signed-in users shouldn't remain (e.g., landing page).
 */
export function AuthRedirect() {
  const { data: session, isPending } = useSession()
  const router = useRouter()

  useEffect(() => {
    if (!isPending && session) {
      router.push("/dashboard")
    }
  }, [isPending, session, router])

  return null
}


"use client"

import { useSession } from "@/hooks/use-session"
import { useRouter } from "next/navigation"
import { useEffect } from "react"

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


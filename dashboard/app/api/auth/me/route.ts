import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import { RELAY_API_URL } from '@/lib/config'

const SESSION_COOKIE = 'relay_session'

type UserResponse = {
  id: string
  name: string
  email: string
  avatar_url?: string
}

type ValidateResponse = {
  user: UserResponse
  has_organization: boolean
}

export async function GET() {
  const cookieStore = await cookies()
  const sessionId = cookieStore.get(SESSION_COOKIE)?.value

  if (!sessionId) {
    return NextResponse.json({ error: 'not authenticated' }, { status: 401 })
  }

  try {
    const res = await fetch(`${RELAY_API_URL}/auth/validate`, {
      headers: {
        'X-Session-ID': sessionId,
      },
    })

    if (!res.ok) {
      if (res.status === 401) {
        cookieStore.delete(SESSION_COOKIE)
        return NextResponse.json({ error: 'session expired' }, { status: 401 })
      }
      return NextResponse.json({ error: 'validation failed' }, { status: res.status })
    }

    const data: ValidateResponse = await res.json()
    return NextResponse.json({ user: data.user, has_organization: data.has_organization })
  } catch (error) {
    console.error('Error validating session:', error)
    return NextResponse.json({ error: 'internal error' }, { status: 500 })
  }
}

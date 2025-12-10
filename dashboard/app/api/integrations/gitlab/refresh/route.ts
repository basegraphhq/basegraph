import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import { RELAY_API_URL } from '@/lib/config'

const SESSION_COOKIE = 'relay_session'

type ValidateResponse = {
  workspace_id?: string
}

export async function POST() {
  try {
    const cookieStore = await cookies()
    const sessionId = cookieStore.get(SESSION_COOKIE)?.value

    if (!sessionId) {
      return NextResponse.json({ error: 'Not authenticated' }, { status: 401 })
    }

    const validateResponse = await fetch(`${RELAY_API_URL}/auth/validate`, {
      headers: {
        'X-Session-ID': sessionId,
      },
    })

    if (!validateResponse.ok) {
      return NextResponse.json({ error: 'Session invalid' }, { status: 401 })
    }

    const validateData: ValidateResponse = await validateResponse.json()
    if (!validateData.workspace_id) {
      return NextResponse.json({ error: 'Workspace missing' }, { status: 400 })
    }

    const refreshResponse = await fetch(`${RELAY_API_URL}/api/v1/integrations/gitlab/refresh`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': sessionId,
      },
      body: JSON.stringify({
        workspace_id: validateData.workspace_id,
      }),
    })

    if (!refreshResponse.ok) {
      const errorData = await refreshResponse.json().catch(() => ({}))
      return NextResponse.json(
        { error: errorData.error || 'Failed to refresh GitLab integration' },
        { status: refreshResponse.status }
      )
    }

    const data = await refreshResponse.json()
    return NextResponse.json(data)
  } catch (error) {
    console.error('Error refreshing GitLab integration:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}

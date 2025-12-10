import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import { RELAY_API_URL } from '@/lib/config'

const SESSION_COOKIE = 'relay_session'

type ValidateResponse = {
  workspace_id?: string
}

export async function GET(req: Request) {
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

    const statusResponse = await fetch(
      `${RELAY_API_URL}/api/v1/integrations/gitlab/status?workspace_id=${validateData.workspace_id}`,
      {
        headers: {
          'X-Session-ID': sessionId,
        },
        cache: 'no-store',
      }
    )

    if (!statusResponse.ok) {
      const errorData = await statusResponse.json().catch(() => ({}))
      return NextResponse.json(
        { error: errorData.error || 'Failed to fetch GitLab status' },
        { status: statusResponse.status }
      )
    }

    const data = await statusResponse.json()
    return NextResponse.json(data)
  } catch (error) {
    console.error('Error fetching GitLab status:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}

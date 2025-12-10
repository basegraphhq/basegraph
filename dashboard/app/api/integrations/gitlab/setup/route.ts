import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import { RELAY_API_URL } from '@/lib/config'

const SESSION_COOKIE = 'relay_session'

type ValidateResponse = {
  user: {
    id: string
    name: string
    email: string
    avatar_url?: string
  }
  has_organization: boolean
  organization_id?: string
  workspace_id?: string
}

type SetupRequest = {
  instance_url: string
  token: string
}

type SetupResponse = {
  integration_id: string
  is_new_integration: boolean
  projects: Array<{
    id: number
    name: string
    path_with_namespace: string
    web_url: string
    description?: string
  }>
  webhooks_created: number
  repositories_added: number
  errors?: string[]
}

export async function POST(req: Request) {
  try {
    const cookieStore = await cookies()
    const sessionId = cookieStore.get(SESSION_COOKIE)?.value

    if (!sessionId) {
      return NextResponse.json(
        { error: 'Not authenticated' },
        { status: 401 }
      )
    }

    const validateResponse = await fetch(`${RELAY_API_URL}/auth/validate`, {
      headers: {
        'X-Session-ID': sessionId,
      },
    })

    if (!validateResponse.ok) {
      return NextResponse.json(
        { error: 'Session invalid' },
        { status: 401 }
      )
    }

    const validateData: ValidateResponse = await validateResponse.json()
    
    if (!validateData.organization_id || !validateData.workspace_id) {
      return NextResponse.json(
        { error: 'Organization setup required' },
        { status: 400 }
      )
    }

    const body: SetupRequest = await req.json()

    if (!body.instance_url || !body.token) {
      return NextResponse.json(
        { error: 'instance_url and token are required' },
        { status: 400 }
      )
    }

    const setupResponse = await fetch(`${RELAY_API_URL}/api/v1/integrations/gitlab/setup`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': sessionId,
      },
      body: JSON.stringify({
        instance_url: body.instance_url,
        token: body.token,
        workspace_id: validateData.workspace_id,
        organization_id: validateData.organization_id,
        setup_by_user_id: validateData.user.id,
      }),
    })

    if (!setupResponse.ok) {
      const errorData = await setupResponse.json().catch(() => ({}))
      console.error('GitLab setup failed:', errorData)
      return NextResponse.json(
        { error: errorData.error || 'Failed to setup GitLab integration' },
        { status: setupResponse.status }
      )
    }

    const setupData: SetupResponse = await setupResponse.json()
    return NextResponse.json(setupData)
  } catch (error) {
    console.error('Error setting up GitLab integration:', error)
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}

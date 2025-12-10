import { NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import { RELAY_API_URL } from '@/lib/config'

const SESSION_COOKIE = 'relay_session'

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

    const user = await validateResponse.json()

    const { name, slug } = await req.json()

    if (!name) {
      return NextResponse.json(
        { error: 'Organization name is required' },
        { status: 400 }
      )
    }

    const createOrgResponse = await fetch(`${RELAY_API_URL}/api/v1/organizations`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-ID': sessionId,
      },
      body: JSON.stringify({
        name,
        slug,
        admin_user_id: user.id,
      }),
    })

    if (!createOrgResponse.ok) {
      const errorText = await createOrgResponse.text()
      return NextResponse.json(
        { error: 'Failed to create organization', details: errorText },
        { status: createOrgResponse.status }
      )
    }

    const orgData = await createOrgResponse.json()

    const res = NextResponse.json(orgData)
    res.cookies.set('relay-onboarding-complete', 'true', {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 365,
      path: '/',
    })

    return res
  } catch (error) {
    console.error('Error creating organization:', error)
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}

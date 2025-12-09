import { NextResponse } from 'next/server'
import { auth } from '@/lib/auth'
import { headers } from 'next/headers'
import { RELAY_API_URL } from '@/lib/config'

export async function POST() {
  try {
    const session = await auth.api.getSession({
      headers: await headers(),
    })

    if (!session?.user) {
      return NextResponse.json(
        { error: 'Not authenticated' },
        { status: 401 }
      )
    }

    const { email, name, image } = session.user

    const response = await fetch(`${RELAY_API_URL}/api/v1/auth/sync`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            name: name || '',
            email: email,
            avatar_url: image
        })
    })

    if (!response.ok) {
        console.error('Relay sync error:', await response.text())
        return NextResponse.json(
            { error: 'Failed to sync user with Relay' },
            { status: response.status }
        )
    }

    const data = await response.json()
    
    // Set onboarding cookie based on organization status
    const hasOrg = data.has_organization === true
    const res = NextResponse.json(data)
    
    res.cookies.set('relay-onboarding-complete', String(hasOrg), {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 365, // 1 year
      path: '/',
    })
    
    return res

  } catch (error) {
    console.error('Error syncing user:', error)
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}

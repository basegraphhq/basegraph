import { NextResponse } from 'next/server'
import { auth } from '@/lib/auth'
import { headers } from 'next/headers'
import { RELAY_API_URL } from '@/lib/config'

export async function POST(req: Request) {
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

    const { name, slug } = await req.json()

    if (!name) {
         return NextResponse.json(
        { error: 'Organization name is required' },
        { status: 400 }
      )
    }

    // Step 1: Sync user to get ID
    const userSyncResponse = await fetch(`${RELAY_API_URL}/api/v1/auth/sync`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            name: session.user.name || '',
            email: session.user.email,
            avatar_url: session.user.image
        })
    })

    if (!userSyncResponse.ok) {
         return NextResponse.json(
            { error: 'Failed to resolve user' },
            { status: 500 }
        )
    }

    const userData = await userSyncResponse.json()
    const relayUserId = userData.user.id

    // Step 2: Create Organization
    const createOrgResponse = await fetch(`${RELAY_API_URL}/api/v1/organizations`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            name,
            slug,
            admin_user_id: relayUserId
        })
    })

    if (!createOrgResponse.ok) {
        const errorText = await createOrgResponse.text()
         return NextResponse.json(
            { error: 'Failed to create organization', details: errorText },
            { status: createOrgResponse.status }
        )
    }

    const orgData = await createOrgResponse.json()
    
    // Set onboarding cookie to mark organization creation complete
    const res = NextResponse.json(orgData)
    res.cookies.set('relay-onboarding-complete', 'true', {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 365, // 1 year
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

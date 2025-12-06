import { NextRequest, NextResponse } from 'next/server'
import { auth } from '@/lib/auth'
import { headers } from 'next/headers'

/**
 * Initiates GitLab OAuth flow with expanded scopes for repository access
 * This is separate from the sign-in flow which only needs read_user scope
 * 
 * Accepts optional `issuer` query param for self-hosted GitLab instances
 * Example: /api/gitlab/authorize?issuer=https://gitlab.mycompany.com
 */
export async function GET(request: NextRequest) {
  try {
    // Check if user is authenticated first
    const session = await auth.api.getSession({
      headers: await headers(),
    })

    if (!session?.user) {
      return NextResponse.json(
        { error: 'Not authenticated. Please sign in first.' },
        { status: 401 }
      )
    }

    const clientId = process.env.GITLAB_CLIENT_ID
    
    // Get issuer from query params, fallback to env var, then default to gitlab.com
    const searchParams = request.nextUrl.searchParams
    const issuer = searchParams.get('issuer') || process.env.GITLAB_ISSUER || 'https://gitlab.com'
    
    const redirectUri = `${process.env.NEXT_PUBLIC_APP_URL}/api/gitlab/callback`

    if (!clientId) {
      return NextResponse.json(
        { error: 'GitLab client not configured' },
        { status: 500 }
      )
    }

    // Generate a random state for CSRF protection
    const state = crypto.randomUUID()

    // Build GitLab OAuth URL with expanded scopes
    const scopes = ['api', 'read_repository']
    const authUrl = new URL(`${issuer}/oauth/authorize`)
    authUrl.searchParams.set('client_id', clientId)
    authUrl.searchParams.set('redirect_uri', redirectUri)
    authUrl.searchParams.set('response_type', 'code')
    authUrl.searchParams.set('scope', scopes.join(' '))
    authUrl.searchParams.set('state', state)

    // Store state in a cookie for verification
    const response = NextResponse.redirect(authUrl.toString())
    response.cookies.set('gitlab_oauth_state', state, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 10, // 10 minutes
      path: '/',
    })
    
    // Store the issuer URL so callback knows which GitLab server to use
    response.cookies.set('gitlab_issuer', issuer, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 10, // 10 minutes
      path: '/',
    })

    return response
  } catch (error) {
    console.error('Error initiating GitLab OAuth:', error)
    return NextResponse.json(
      { error: 'Failed to initiate OAuth flow' },
      { status: 500 }
    )
  }
}


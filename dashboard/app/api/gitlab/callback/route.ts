import { NextRequest, NextResponse } from 'next/server'
import { auth } from '@/lib/auth'
import { headers } from 'next/headers'

/**
 * Handles GitLab OAuth callback
 * Exchanges the authorization code for an access token and stores it
 */
export async function GET(request: NextRequest) {
  try {
    // Check if user is authenticated
    const session = await auth.api.getSession({
      headers: await headers(),
    })

    if (!session?.user) {
      return NextResponse.redirect(new URL('/?error=not_authenticated', request.url))
    }

    const searchParams = request.nextUrl.searchParams
    const code = searchParams.get('code')
    const state = searchParams.get('state')
    const error = searchParams.get('error')
    const errorDescription = searchParams.get('error_description')

    // Check for OAuth errors
    if (error) {
      console.error('GitLab OAuth error:', error, errorDescription)
      return NextResponse.redirect(
        new URL(`/dashboard?gitlab_error=${encodeURIComponent(errorDescription || error)}`, request.url)
      )
    }

    // Verify state to prevent CSRF
    const storedState = request.cookies.get('gitlab_oauth_state')?.value
    if (!state || state !== storedState) {
      console.error('State mismatch:', { state, storedState })
      return NextResponse.redirect(
        new URL('/dashboard?gitlab_error=invalid_state', request.url)
      )
    }

    if (!code) {
      return NextResponse.redirect(
        new URL('/dashboard?gitlab_error=no_code', request.url)
      )
    }

    // Exchange code for access token
    const clientId = process.env.GITLAB_CLIENT_ID
    const clientSecret = process.env.GITLAB_CLIENT_SECRET
    
    // Get issuer from cookie (set during authorize), fallback to env var or default
    const issuer = request.cookies.get('gitlab_issuer')?.value 
      || process.env.GITLAB_ISSUER 
      || 'https://gitlab.com'
    
    const redirectUri = `${process.env.NEXT_PUBLIC_APP_URL}/api/gitlab/callback`

    if (!clientId || !clientSecret) {
      return NextResponse.redirect(
        new URL('/dashboard?gitlab_error=server_config', request.url)
      )
    }

    const tokenResponse = await fetch(`${issuer}/oauth/token`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        client_id: clientId,
        client_secret: clientSecret,
        code,
        grant_type: 'authorization_code',
        redirect_uri: redirectUri,
      }),
    })

    if (!tokenResponse.ok) {
      const errorData = await tokenResponse.text()
      console.error('Token exchange failed:', errorData)
      return NextResponse.redirect(
        new URL('/dashboard?gitlab_error=token_exchange_failed', request.url)
      )
    }

    const tokenData = await tokenResponse.json()
    const { access_token, refresh_token, expires_in } = tokenData

    // Store the access token in a secure cookie
    // In production, you'd want to store this in a database linked to the user
    const response = NextResponse.redirect(
      new URL('/dashboard?gitlab_connected=true', request.url)
    )

    // Clear the temporary OAuth cookies
    response.cookies.delete('gitlab_oauth_state')
    response.cookies.delete('gitlab_issuer')
    
    // Store the GitLab instance URL for future API calls
    response.cookies.set('gitlab_instance_url', issuer, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 30, // 30 days
      path: '/',
    })

    // Store the GitLab token (encrypted in production)
    // For now, we store it in an HTTP-only cookie
    response.cookies.set('gitlab_access_token', access_token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: expires_in || 7200, // Default 2 hours
      path: '/',
    })

    if (refresh_token) {
      response.cookies.set('gitlab_refresh_token', refresh_token, {
        httpOnly: true,
        secure: process.env.NODE_ENV === 'production',
        sameSite: 'lax',
        maxAge: 60 * 60 * 24 * 30, // 30 days
        path: '/',
      })
    }

    return response
  } catch (error) {
    console.error('Error in GitLab callback:', error)
    return NextResponse.redirect(
      new URL('/dashboard?gitlab_error=callback_failed', request.url)
    )
  }
}


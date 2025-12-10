import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { RELAY_API_URL } from '@/lib/config'

const ONBOARDING_COOKIE = 'relay-onboarding-complete'
const SESSION_COOKIE = 'relay_session'

export async function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl

  const sessionId = request.cookies.get(SESSION_COOKIE)?.value
  const hasOrgCookie =
    request.cookies.get(ONBOARDING_COOKIE)?.value === 'true'

  if (!sessionId) {
    if (pathname.startsWith('/dashboard')) {
      return NextResponse.redirect(new URL('/', request.url))
    }
    return NextResponse.next()
  }

  let hasOrganization = hasOrgCookie
  let responseToSetCookie: NextResponse | undefined

  if (!hasOrgCookie) {
    const derived = await fetchHasOrganization(sessionId)
    if (derived === null) {
      return NextResponse.redirect(new URL('/', request.url))
    }
    hasOrganization = derived
  }

  if (pathname === '/') {
    const target = hasOrganization ? '/dashboard' : '/dashboard/onboarding'
    responseToSetCookie = NextResponse.redirect(new URL(target, request.url))
  }

  if (!responseToSetCookie && pathname.startsWith('/dashboard') && pathname !== '/dashboard/onboarding') {
    if (!hasOrganization) {
      responseToSetCookie = NextResponse.redirect(
        new URL('/dashboard/onboarding', request.url),
      )
    }
  }

  if (!responseToSetCookie && pathname === '/dashboard/onboarding') {
    if (hasOrganization) {
      responseToSetCookie = NextResponse.redirect(
        new URL('/dashboard', request.url),
      )
    }
  }

  const res = responseToSetCookie ?? NextResponse.next()

  if (!hasOrgCookie) {
    res.cookies.set(ONBOARDING_COOKIE, String(hasOrganization), {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 365,
      path: '/',
    })
  }

  return res
}

async function fetchHasOrganization(sessionId: string): Promise<boolean | null> {
  try {
    const res = await fetch(`${RELAY_API_URL}/auth/validate`, {
      headers: {
        'X-Session-ID': sessionId,
      },
    })

    if (!res.ok) {
      return null
    }

    const data = await res.json()
    return data.has_organization
  } catch {
    return null
  }
}

export const config = {
  matcher: ['/', '/dashboard/:path*'],
}

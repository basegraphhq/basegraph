import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'
import { fetchHasOrganization } from '@/lib/relay-client'

const ONBOARDING_COOKIE = 'relay-onboarding-complete'
const SESSION_COOKIES = [
  'better-auth.session_token',
  'better-auth.session',
  'auth_session',
]

export async function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl

  const hasSession = SESSION_COOKIES.some(
    (name) => request.cookies.get(name)?.value,
  )
  const hasOrgCookie =
    request.cookies.get(ONBOARDING_COOKIE)?.value === 'true'

  // No session: treat as logged out
  if (!hasSession) {
    if (pathname.startsWith('/dashboard')) {
      return NextResponse.redirect(new URL('/', request.url))
    }
    return NextResponse.next()
  }

  // Session present, but missing onboarding cookie: re-derive from backend
  let hasOrganization = hasOrgCookie
  let responseToSetCookie: NextResponse | undefined

  if (!hasOrgCookie) {
    const derived = await fetchHasOrganization({
      baseUrl: request.nextUrl.origin,
      headers: {
        cookie: request.headers.get('cookie') ?? '',
      },
    })
    if (derived === null) {
      // If we cannot determine, fall back to logged-out behavior
      return NextResponse.redirect(new URL('/', request.url))
    }
    hasOrganization = derived
  }

  // Landing page: redirect authenticated users appropriately
  if (pathname === '/') {
    const target = hasOrganization ? '/dashboard' : '/dashboard/onboarding'
    responseToSetCookie = NextResponse.redirect(new URL(target, request.url))
  }

  // Dashboard pages (exclude onboarding page itself)
  if (!responseToSetCookie && pathname.startsWith('/dashboard') && pathname !== '/dashboard/onboarding') {
    if (!hasOrganization) {
      responseToSetCookie = NextResponse.redirect(
        new URL('/dashboard/onboarding', request.url),
      )
    }
  }

  // If on onboarding page but already has organization, redirect to dashboard
  if (!responseToSetCookie && pathname === '/dashboard/onboarding') {
    if (hasOrganization) {
      responseToSetCookie = NextResponse.redirect(
        new URL('/dashboard', request.url),
      )
    }
  }

  const res = responseToSetCookie ?? NextResponse.next()

  // If we re-derived org status, persist the cookie for speed next time
  if (!hasOrgCookie) {
    res.cookies.set(ONBOARDING_COOKIE, String(hasOrganization), {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 60 * 60 * 24 * 365, // 1 year
      path: '/',
    })
  }

  return res
}

export const config = {
  matcher: ['/', '/dashboard/:path*'],
}

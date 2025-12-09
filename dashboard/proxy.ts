import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'

export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl
  const hasOrganization =
    request.cookies.get('relay-onboarding-complete')?.value === 'true'

  // Landing page: if authenticated, send to dashboard or onboarding
  if (pathname === '/') {
    const sessionCookie =
      request.cookies.get('better-auth.session_token') ??
      request.cookies.get('better-auth.session') ??
      request.cookies.get('auth_session')

    if (sessionCookie?.value) {
      const target = hasOrganization ? '/dashboard' : '/dashboard/onboarding'
      return NextResponse.redirect(new URL(target, request.url))
    }
  }

  // Dashboard pages (exclude onboarding page itself)
  if (pathname.startsWith('/dashboard') && pathname !== '/dashboard/onboarding') {
    if (!hasOrganization) {
      return NextResponse.redirect(new URL('/dashboard/onboarding', request.url))
    }
  }

  // If on onboarding page but already has organization, redirect to dashboard
  if (pathname === '/dashboard/onboarding') {
    if (hasOrganization) {
      return NextResponse.redirect(new URL('/dashboard', request.url))
    }
  }

  return NextResponse.next()
}

export const config = {
  matcher: ['/', '/dashboard/:path*'],
}

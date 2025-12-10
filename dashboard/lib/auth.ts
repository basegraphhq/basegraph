export type User = {
  id: string
  name: string
  email: string
  avatar_url?: string
}

export type Session = {
  user: User
}

export async function getSession(): Promise<Session | null> {
  try {
    const res = await fetch('/api/auth/me', {
      credentials: 'include',
    })

    if (!res.ok) {
      return null
    }

    const data = await res.json()
    return { user: data.user }
  } catch {
    return null
  }
}

export async function signOut(): Promise<void> {
  await fetch('/api/auth/logout', {
    method: 'POST',
    credentials: 'include',
  })
}

export function getLoginUrl(): string {
  return '/api/auth/login'
}

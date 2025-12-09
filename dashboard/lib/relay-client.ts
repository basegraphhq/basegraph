const RELAY_API_BASE =
  (process.env.RELAY_API_URL ?? 'http://localhost:8080').replace(/\/$/, '')

type GitLabTestResponse = {
  username: string
  project_count: number
}

export async function testGitLabConnection(payload: {
  instance_url: string
  token: string
}): Promise<GitLabTestResponse> {
  const res = await fetch(`${RELAY_API_BASE}/api/v1/integrations/gitlab/test-connection`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  })

  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `relay error ${res.status}`)
  }

  return res.json()
}

/**
 * Fetches onboarding status from the dashboard's sync endpoint.
 * Returns true/false when determinable, or null on failure.
 */
export async function fetchHasOrganization(options?: {
  baseUrl?: string
  headers?: HeadersInit
}): Promise<boolean | null> {
  const { baseUrl = '', headers } = options ?? {}
  const url = `${baseUrl}/api/user/sync`

  try {
    const res = await fetch(url, {
      method: 'POST',
      headers,
    })

    if (!res.ok) {
      return null
    }

    const data = await res.json()
    return data?.has_organization === true
  } catch {
    return null
  }
}

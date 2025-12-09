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

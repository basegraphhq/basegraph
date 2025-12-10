import { RELAY_API_URL } from './config'

type GitLabTestResponse = {
  username: string
  project_count: number
}

export async function testGitLabConnection(payload: {
  instance_url: string
  token: string
}): Promise<GitLabTestResponse> {
  const res = await fetch(`${RELAY_API_URL}/api/v1/integrations/gitlab/test-connection`, {
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

export type GitLabProject = {
  id: number
  name: string
  path_with_namespace: string
  web_url: string
  description?: string
}

export type GitLabSetupResponse = {
  integration_id: string
  is_new_integration: boolean
  projects: GitLabProject[]
  webhooks_created: number
  repositories_added: number
  errors?: string[]
}

export type GitLabSetupParams = {
  instanceUrl: string
  token: string
}

export type GitLabStatusResponse = {
  connected: boolean
  integration_id?: string
  status?: {
    synced: boolean
    webhooks_created: number
    repositories_added: number
    errors?: string[]
    updated_at?: string
  }
  repos_count: number
}

export type GitLabRefreshResponse = {
  integration_id: string
  is_new_integration: boolean
  projects: GitLabProject[]
  webhooks_created: number
  repositories_added: number
  errors?: string[]
  synced: boolean
}

class RelayClientError extends Error {
  constructor(
    message: string,
    public status: number
  ) {
    super(message)
    this.name = 'RelayClientError'
  }
}

function formatErrorMessage(error: string, status: number): string {
  if (error.includes('no projects found with maintainer access')) {
    return error
  }
  if (error.includes('validating token') || error.includes('401') || status === 401) {
    if (status === 401 && !error.includes('token')) {
      return 'Session expired. Please sign in again.'
    }
    return error || 'Invalid token. Please check your Personal Access Token and try again.'
  }
  if (status === 403) {
    return error || 'Token does not have sufficient permissions. Ensure the token has \'api\' scope and the user has Maintainer access to at least one project.'
  }
  if (error.includes('Organization setup required')) {
    return 'Please complete organization setup before connecting integrations.'
  }
  if (status === 400) {
    return error || 'Invalid request. Please check your input and try again.'
  }
  if (status === 502 || status === 503) {
    return 'Unable to connect to GitLab. Please check the instance URL and try again.'
  }
  return error || 'Something went wrong. Please try again.'
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    const message = formatErrorMessage(data.error || '', response.status)
    throw new RelayClientError(message, response.status)
  }
  return response.json()
}

export const relayClient = {
  gitlab: {
    async setup(params: GitLabSetupParams): Promise<GitLabSetupResponse> {
      const response = await fetch('/api/integrations/gitlab/setup', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include',
        body: JSON.stringify({
          instance_url: params.instanceUrl,
          token: params.token,
        }),
      })
      return handleResponse<GitLabSetupResponse>(response)
    },
    async status(): Promise<GitLabStatusResponse> {
      const response = await fetch('/api/integrations/gitlab/status', {
        method: 'GET',
        credentials: 'include',
        cache: 'no-store',
      })
      return handleResponse<GitLabStatusResponse>(response)
    },
    async refresh(): Promise<GitLabRefreshResponse> {
      const response = await fetch('/api/integrations/gitlab/refresh', {
        method: 'POST',
        credentials: 'include',
      })
      return handleResponse<GitLabRefreshResponse>(response)
    },
  },
}

export { RelayClientError }

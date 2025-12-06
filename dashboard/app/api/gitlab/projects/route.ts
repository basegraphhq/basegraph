import { NextRequest, NextResponse } from 'next/server'
import { auth } from '@/lib/auth'
import { headers } from 'next/headers'

/**
 * Fetches GitLab projects using the stored access token
 */
export async function GET(request: NextRequest) {
  try {
    // Check if user is authenticated
    const session = await auth.api.getSession({
      headers: await headers(),
    })

    if (!session?.user) {
      return NextResponse.json(
        { error: 'Not authenticated' },
        { status: 401 }
      )
    }

    // Get the GitLab access token from cookie
    const accessToken = request.cookies.get('gitlab_access_token')?.value

    if (!accessToken) {
      return NextResponse.json(
        { error: 'GitLab not connected. Please connect your GitLab account first.' },
        { status: 403 }
      )
    }

    // Get the GitLab instance URL (stored during OAuth callback)
    const issuer = request.cookies.get('gitlab_instance_url')?.value 
      || process.env.GITLAB_ISSUER 
      || 'https://gitlab.com'

    // Fetch projects from GitLab API
    const projectsResponse = await fetch(
      `${issuer}/api/v4/projects?membership=true&per_page=100&order_by=last_activity_at`,
      {
        headers: {
          Authorization: `Bearer ${accessToken}`,
        },
      }
    )

    if (!projectsResponse.ok) {
      // If token is invalid, clear all GitLab cookies
      if (projectsResponse.status === 401) {
        const response = NextResponse.json(
          { error: 'GitLab token expired. Please reconnect your GitLab account.' },
          { status: 401 }
        )
        response.cookies.delete('gitlab_access_token')
        response.cookies.delete('gitlab_refresh_token')
        response.cookies.delete('gitlab_instance_url')
        return response
      }

      const errorText = await projectsResponse.text()
      console.error('GitLab API error:', errorText)
      return NextResponse.json(
        { error: 'Failed to fetch projects from GitLab' },
        { status: 500 }
      )
    }

    const projects = await projectsResponse.json()

    // Return simplified project data
    const simplifiedProjects = projects.map((project: {
      id: number
      name: string
      path_with_namespace: string
      description: string | null
      web_url: string
      default_branch: string
      visibility: string
      last_activity_at: string
      namespace: {
        name: string
        path: string
      }
    }) => ({
      id: project.id,
      name: project.name,
      fullPath: project.path_with_namespace,
      description: project.description,
      url: project.web_url,
      defaultBranch: project.default_branch,
      visibility: project.visibility,
      lastActivityAt: project.last_activity_at,
      namespace: project.namespace?.name || project.namespace?.path,
    }))

    return NextResponse.json({
      success: true,
      count: simplifiedProjects.length,
      projects: simplifiedProjects,
    })
  } catch (error) {
    console.error('Error fetching GitLab projects:', error)
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}


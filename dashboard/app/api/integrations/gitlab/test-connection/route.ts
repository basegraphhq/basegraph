import { NextResponse } from 'next/server'
import { testGitLabConnection } from '@/lib/relay-client'

type Body = {
  instance_url?: string
  token?: string
}

export async function POST(request: Request) {
  const { instance_url, token }: Body = await request.json()

  if (!instance_url || !token || instance_url.length < 10 || token.length < 10) {
    return NextResponse.json({ error: 'invalid request' }, { status: 400 })
  }

  try {
    const result = await testGitLabConnection({
      instance_url,
      token,
    })
    return NextResponse.json(result)
  } catch (error) {
    console.error('Relay GitLab test failed', error)
    return NextResponse.json({ error: 'relay request failed' }, { status: 502 })
  }
}

export async function OPTIONS() {
  return NextResponse.json({}, { status: 200 })
}

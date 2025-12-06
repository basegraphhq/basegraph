import { NextResponse } from 'next/server'
import { auth } from '@/lib/auth'
import { headers } from 'next/headers'

export async function POST() {
  try {
    // Get the current session
    const session = await auth.api.getSession({
      headers: await headers(),
    })

    if (!session?.user) {
      return NextResponse.json(
        { error: 'Not authenticated' },
        { status: 401 }
      )
    }

    const { email, name } = session.user

    if (!email) {
      return NextResponse.json(
        { error: 'User email not available' },
        { status: 400 }
      )
    }

    // Get Airtable credentials
    const airtableApiKey = process.env.AIRTABLE_API_KEY
    const airtableBaseId = process.env.AIRTABLE_BASE_ID
    const airtableTableName = process.env.AIRTABLE_TABLE_NAME || 'Waitlist'

    if (!airtableApiKey || !airtableBaseId) {
      console.error('Missing Airtable configuration')
      return NextResponse.json(
        { error: 'Server configuration error' },
        { status: 500 }
      )
    }

    // Check if user already exists in Airtable
    const checkResponse = await fetch(
      `https://api.airtable.com/v0/${airtableBaseId}/${encodeURIComponent(airtableTableName)}?filterByFormula=${encodeURIComponent(`{Email}="${email}"`)}`,
      {
        method: 'GET',
        headers: {
          Authorization: `Bearer ${airtableApiKey}`,
        },
      }
    )

    if (!checkResponse.ok) {
      const errorText = await checkResponse.text()
      console.error('Airtable check error:', {
        status: checkResponse.status,
        statusText: checkResponse.statusText,
        body: errorText,
        url: `https://api.airtable.com/v0/${airtableBaseId}/${airtableTableName}`,
      })
      return NextResponse.json(
        { 
          error: 'Failed to check existing user',
          details: errorText,
          status: checkResponse.status,
        },
        { status: 500 }
      )
    }

    const checkData = await checkResponse.json()

    // If user already exists, return success (idempotent)
    if (checkData.records && checkData.records.length > 0) {
      return NextResponse.json({
        success: true,
        message: 'User already exists',
        id: checkData.records[0].id,
      })
    }

    // Create new record in Airtable
    const createResponse = await fetch(
      `https://api.airtable.com/v0/${airtableBaseId}/${encodeURIComponent(airtableTableName)}`,
      {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${airtableApiKey}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          fields: {
            Email: email,
            Name: name || '',
            'Joined At': new Date().toISOString(),
            Source: 'Dashboard',
          },
        }),
      }
    )

    if (!createResponse.ok) {
      const errorData = await createResponse.json().catch(() => ({}))
      console.error('Airtable create error:', errorData)
      return NextResponse.json(
        { error: 'Failed to add user to Airtable' },
        { status: 500 }
      )
    }

    const data = await createResponse.json()

    return NextResponse.json({
      success: true,
      message: 'User added',
      id: data.id,
    })
  } catch (error) {
    console.error('Error syncing user:', error)
    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}


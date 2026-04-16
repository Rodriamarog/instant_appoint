import { NextRequest, NextResponse } from 'next/server'

const VERIFY_TOKEN = 'neurocrow_webhook_verification'

export async function handleWebhookGet(request: NextRequest) {
  const { searchParams } = new URL(request.url)

  const mode = searchParams.get('hub.mode')
  const token = searchParams.get('hub.verify_token')
  const challenge = searchParams.get('hub.challenge')

  if (mode === 'subscribe' && token === VERIFY_TOKEN) {
    console.log('Facebook webhook verified')
    return new NextResponse(challenge, { status: 200 })
  }

  return NextResponse.json({ error: 'Forbidden' }, { status: 403 })
}

export async function handleWebhookPost(request: NextRequest) {
  try {
    const body = await request.json()
    console.log('Facebook webhook event:', JSON.stringify(body, null, 2))

    // TODO: handle specific event types here

    return NextResponse.json({ status: 'ok' }, { status: 200 })
  } catch (error) {
    console.error('Error processing webhook:', error)
    return NextResponse.json({ error: 'Internal server error' }, { status: 500 })
  }
}

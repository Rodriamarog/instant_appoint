import { NextRequest, NextResponse } from 'next/server'
import PocketBase from 'pocketbase'
import { sendWhatsAppTemplate } from '@/lib/whatsapp-cloud-api'

const PB_URL = process.env.NEXT_PUBLIC_POCKETBASE_URL || 'http://127.0.0.1:8090'
const PB_ADMIN_EMAIL = process.env.POCKETBASE_ADMIN_EMAIL!
const PB_ADMIN_PASSWORD = process.env.POCKETBASE_ADMIN_PASSWORD!

export async function POST(request: NextRequest) {
  const authHeader = request.headers.get('Authorization')
  if (!authHeader?.startsWith('Bearer ')) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  }

  const userPb = new PocketBase(PB_URL)
  userPb.authStore.save(authHeader.substring(7))
  try {
    await userPb.collection('users').authRefresh()
  } catch {
    return NextResponse.json({ error: 'Invalid auth token' }, { status: 401 })
  }
  const userId = userPb.authStore.model?.id as string

  const { template_name, language_code, to, body_parameters } = await request.json()
  if (!template_name || !language_code || !to) {
    return NextResponse.json({ error: 'template_name, language_code, and to are required' }, { status: 400 })
  }

  const adminPb = new PocketBase(PB_URL)
  await adminPb.collection('_superusers').authWithPassword(PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD)

  let account
  try {
    account = await adminPb.collection('whatsapp_accounts').getFirstListItem(
      `user_id = "${userId}" && account_type = "business_api" && is_active = true`
    )
  } catch {
    return NextResponse.json({ error: 'No connected WhatsApp account found' }, { status: 404 })
  }

  const wamid = await sendWhatsAppTemplate(
    account.phone_number_id as string,
    account.access_token as string,
    to,
    template_name,
    language_code,
    body_parameters ?? []
  )

  return NextResponse.json({ success: true, message_id: wamid })
}

/**
 * Run once to create the calendar_events collection in PocketBase.
 * Usage: node scripts/setup-pocketbase.js
 *
 * Make sure PocketBase is running first: ./pocketbase serve
 * You'll need your admin email/password (set when you first accessed /_/)
 */

const PB_URL = process.env.PUBLIC_POCKETBASE_URL || 'http://127.0.0.1:8090'
const ADMIN_EMAIL = process.env.PB_ADMIN_EMAIL
const ADMIN_PASSWORD = process.env.PB_ADMIN_PASSWORD

async function main() {
  if (!ADMIN_EMAIL || !ADMIN_PASSWORD) {
    console.error('Set PB_ADMIN_EMAIL and PB_ADMIN_PASSWORD env vars before running.')
    console.error('Example: PB_ADMIN_EMAIL=you@example.com PB_ADMIN_PASSWORD=yourpass node scripts/setup-pocketbase.js')
    process.exit(1)
  }

  // Authenticate as admin
  const authRes = await fetch(`${PB_URL}/api/admins/auth-with-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identity: ADMIN_EMAIL, password: ADMIN_PASSWORD }),
  })
  if (!authRes.ok) {
    console.error('Admin auth failed:', await authRes.text())
    process.exit(1)
  }
  const { token } = await authRes.json()
  console.log('Authenticated as admin.')

  const headers = {
    'Content-Type': 'application/json',
    'Authorization': token,
  }

  // Check if collection already exists
  const listRes = await fetch(`${PB_URL}/api/collections/calendar_events`, { headers })
  if (listRes.ok) {
    console.log('calendar_events collection already exists. Nothing to do.')
    return
  }

  // Create the collection
  const createRes = await fetch(`${PB_URL}/api/collections`, {
    method: 'POST',
    headers,
    body: JSON.stringify({
      name: 'calendar_events',
      type: 'base',
      schema: [
        { name: 'user_id',      type: 'text',     required: true },
        { name: 'title',        type: 'text',     required: true },
        { name: 'client_phone', type: 'text',     required: false },
        { name: 'notes',        type: 'text',     required: false },
        { name: 'start_time',   type: 'text',     required: true },
        { name: 'end_time',     type: 'text',     required: true },
        { name: 'location',     type: 'text',     required: false },
      ],
      listRule:   '@request.auth.id != ""',
      viewRule:   '@request.auth.id != ""',
      createRule: '@request.auth.id != ""',
      updateRule: '@request.auth.id != ""',
      deleteRule: '@request.auth.id != ""',
    }),
  })

  if (!createRes.ok) {
    console.error('Failed to create collection:', await createRes.text())
    process.exit(1)
  }

  console.log('calendar_events collection created successfully.')
}

main().catch(console.error)

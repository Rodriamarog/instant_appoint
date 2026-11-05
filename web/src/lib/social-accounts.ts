import { call } from '@/lib/api'
import { pb } from '@/lib/pocketbase'

/**
 * Connected Facebook Pages and Instagram accounts — the social_accounts
 * collection, and the two Go routes that write it.
 *
 * Reading is the browser talking to PocketBase directly, scoped by the list
 * rule; writing is only ever the server, which holds the Meta app secret the
 * login's code is exchanged with. access_token is hidden and never named here.
 */
export const COLLECTION = 'social_accounts'

export type SocialChannel = 'messenger' | 'instagram'

export interface SocialAccount {
  id: string
  channel: SocialChannel
  external_id: string
  page_id: string
  /** The Page's name, or the Instagram account's. */
  name: string
  /** The Instagram handle, without the @. Empty for Messenger. */
  username: string
}

/** The signed-in person's connected Page and Instagram account, by channel. */
export async function listSocialAccounts(): Promise<Partial<Record<SocialChannel, SocialAccount>>> {
  const records = await pb.collection(COLLECTION).getFullList<SocialAccount>({
    filter: pb.filter('user_id = {:userId}', {
      userId: pb.authStore.record?.id ?? '',
    }),
  })

  const byChannel: Partial<Record<SocialChannel, SocialAccount>> = {}
  for (const record of records) {
    byChannel[record.channel] = record
  }

  return byChannel
}

/** A Page offered when the login granted more than one. */
export interface PageChoice {
  id: string
  name: string
  instagram_username?: string
}

export interface Connected {
  page_name: string
  instagram_username?: string
  has_instagram: boolean
}

export type ConnectResult = { connected: Connected } | { pages: PageChoice[] }

const FAILED = 'No se pudo conectar con Facebook. Inténtalo de nuevo.'

/** Hands the server the login's code. */
export function connectWithCode(code: string): Promise<ConnectResult> {
  return call<ConnectResult>(
    '/api/social/connect',
    { method: 'POST', body: JSON.stringify({ code }) },
    FAILED,
  )
}

/** Connects one of the Pages the login offered. */
export function choosePage(pageId: string): Promise<ConnectResult> {
  return call<ConnectResult>(
    '/api/social/connect',
    { method: 'POST', body: JSON.stringify({ page_id: pageId }) },
    FAILED,
  )
}

export function disconnectChannel(channel: SocialChannel): Promise<unknown> {
  return call(
    '/api/social/disconnect',
    { method: 'POST', body: JSON.stringify({ channel }) },
    'No se pudo desconectar. Inténtalo de nuevo.',
  )
}

import PocketBase from 'pocketbase'

/**
 * The SPA is served by the same Go binary that runs PocketBase, so the API
 * lives at the same origin and no base URL has to be configured.
 *
 * In development `vite dev` proxies /api to the Go server, so '/' is correct
 * there too.
 */
export const pb = new PocketBase('/')

// Overlapping requests must not abort each other.
pb.autoCancellation(false)

export default pb

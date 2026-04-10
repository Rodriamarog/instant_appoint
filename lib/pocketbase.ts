import PocketBase from 'pocketbase'

const pb = new PocketBase(process.env.PUBLIC_POCKETBASE_URL || 'http://127.0.0.1:8090')

// Disable auto cancellation
pb.autoCancellation(false)

export { pb }
export default pb
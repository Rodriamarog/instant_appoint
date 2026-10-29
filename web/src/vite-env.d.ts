/// <reference types="vite/client" />

/**
 * The build-time settings this app reads. Both are public identifiers of the
 * Meta app the Embedded Signup runs against, baked into the bundle at build
 * time — see lib/embedded-signup, and the README's environment section.
 */
interface ImportMetaEnv {
  readonly VITE_META_APP_ID?: string
  readonly VITE_META_CONFIG_ID?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

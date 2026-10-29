/**
 * One sentence to put in front of a person when something failed.
 *
 * A `catch` binding is `unknown`, and what lands in it here is usually an
 * Error the PocketBase SDK or lib/api built with a message already written
 * for a reader. Anything else — a thrown string, a rejected non-Error — has
 * no sentence in it worth showing, so the caller's fallback stands in.
 *
 * An Error with an empty message counts as having nothing to say: showing a
 * blank where an explanation belongs reads as a bug.
 */
export function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback
}

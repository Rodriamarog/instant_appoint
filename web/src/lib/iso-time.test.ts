import { expect, test } from 'vitest'

import {
  isoMillis,
  localDateInput,
  localInputsToIso,
  localTimeInput,
} from '@/lib/iso-time'

/**
 * The same instants are pinned in internal/isotime/isotime_test.go. Both sides
 * of the wire write these columns, so both have to produce byte-identical
 * output or the string ordering the app depends on breaks.
 */
const cases: Array<[string, Date, string]> = [
  [
    'a whole second still carries three fractional digits',
    new Date(Date.UTC(2026, 10, 2, 17, 0, 0, 0)),
    '2026-11-02T17:00:00.000Z',
  ],
  [
    'milliseconds are kept',
    new Date(Date.UTC(2026, 10, 2, 17, 0, 0, 123)),
    '2026-11-02T17:00:00.123Z',
  ],
  [
    'a trailing zero millisecond is not trimmed',
    new Date(Date.UTC(2026, 10, 2, 17, 0, 0, 500)),
    '2026-11-02T17:00:00.500Z',
  ],
  [
    'single-digit fields are zero-padded',
    new Date(Date.UTC(2026, 0, 2, 3, 4, 5, 6)),
    '2026-01-02T03:04:05.006Z',
  ],
]

test.each(cases)('%s', (_name, instant, want) => {
  expect(isoMillis(instant)).toBe(want)
})

test('every formatted value is exactly 24 characters', () => {
  // Fixed width is what makes the lexicographic comparison SQLite performs on
  // these text columns a chronological one.
  for (const [, instant] of cases) {
    expect(isoMillis(instant)).toHaveLength(24)
  }
})

test('sorting the strings orders the instants', () => {
  const instants = [
    new Date(Date.UTC(2026, 10, 2, 17, 0, 0, 999)),
    new Date(Date.UTC(2026, 10, 2, 17, 0, 0, 0)),
    new Date(Date.UTC(2026, 10, 2, 17, 0, 0, 450)),
    new Date(Date.UTC(2026, 10, 2, 17, 0, 0, 500)),
    new Date(Date.UTC(2026, 10, 2, 16, 59, 59, 999)),
  ]

  const sortedStrings = instants.map(isoMillis).sort()
  const chronological = [...instants]
    .sort((a, b) => a.getTime() - b.getTime())
    .map(isoMillis)

  expect(sortedStrings).toEqual(chronological)
})

test('an invalid date is refused rather than written as "Invalid Date"', () => {
  expect(() => isoMillis(new Date('nonsense'))).toThrow()
})

test('the date and time a person typed become one UTC instant', () => {
  // The two <input> values are local wall-clock, which is what the person
  // meant; the stored value is the UTC instant it names.
  const iso = localInputsToIso('2026-11-02', '09:30')
  const expected = new Date(2026, 10, 2, 9, 30, 0, 0)

  expect(iso).toBe(isoMillis(expected))
  expect(new Date(iso).getHours()).toBe(9)
})

test('an unparseable date/time pair is refused', () => {
  expect(() => localInputsToIso('', '09:30')).toThrow()
  expect(() => localInputsToIso('2026-11-02', '')).toThrow()
})

test('an instant round-trips through the two inputs unchanged', () => {
  // The old dialog read the date in UTC and the time in local, so an evening
  // appointment reopened and saved jumped a day. Both halves have to come
  // from the same clock.
  const instant = new Date(2026, 10, 2, 23, 30, 0, 0)

  const iso = localInputsToIso(localDateInput(instant), localTimeInput(instant))

  expect(iso).toBe(isoMillis(instant))
})

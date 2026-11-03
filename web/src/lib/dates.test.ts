import { expect, test } from 'vitest'

import { spellDateTime, spellTime } from '@/lib/dates'

/**
 * The seam is the two spelling functions themselves: they are pure, and every
 * date this interface prints goes through one of them.
 *
 * The expected strings below were read off a wall calendar, not produced by
 * the code — 3 November 2026 is a Tuesday, 31 January 2026 a Saturday and 25
 * December 2026 a Friday. A month or weekday index that is one out is the
 * failure these catch.
 */
test('a date names its month, because the digits alone read two ways', () => {
  expect(spellDateTime(new Date(2026, 10, 3, 15, 30))).toBe('mar 3 nov 2026, 15:30')
})

test('January and December survive the ends of the table', () => {
  expect(spellDateTime(new Date(2026, 0, 31, 9, 0))).toBe('sáb 31 ene 2026, 09:00')
  expect(spellDateTime(new Date(2026, 11, 25, 20, 0))).toBe('vie 25 dic 2026, 20:00')
})

test('the clock is 24-hour, so no hour is twelve hours wide', () => {
  expect(spellTime(new Date(2026, 10, 3, 0, 5))).toBe('00:05')
  expect(spellTime(new Date(2026, 10, 3, 13, 0))).toBe('13:00')
})

import { afterAll, beforeAll, expect, test } from 'vitest'

import {
  spellDateTime,
  spellDay,
  spellTime,
  zonedDay,
  zonedTime,
} from '@/lib/dates'

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

/**
 * The clinic's day, not the viewer's.
 *
 * The whole file runs with the process in Madrid — nine hours from Tijuana,
 * and on the other side of midnight from it for most of the evening. Every
 * expectation below is therefore wrong by a day if anything in the
 * implementation reaches for the machine's own zone.
 */
const VIEWER_TZ = process.env.TZ

beforeAll(() => {
  process.env.TZ = 'Europe/Madrid'
})

afterAll(() => {
  process.env.TZ = VIEWER_TZ
})

test('an instant lands on the day the clinic is having, not the one the viewer is', () => {
  // 06:30 UTC on 3 November: still the evening of the 2nd in Tijuana
  // (UTC-8), already the small hours of the 3rd in Mexico City (UTC-6),
  // and 07:30 in the morning of the 3rd where this process thinks it is.
  const instant = new Date('2026-11-03T06:30:00.000Z')

  expect(zonedDay(instant, 'America/Tijuana')).toBe('2026-11-02')
  expect(zonedDay(instant, 'America/Mexico_City')).toBe('2026-11-03')
})

test('the clock on a row is the clinic wall clock', () => {
  const instant = new Date('2026-11-03T06:30:00.000Z')

  expect(zonedTime(instant, 'America/Tijuana')).toBe('22:30')
  expect(zonedTime(instant, 'America/Mexico_City')).toBe('00:30')
})

test('midnight in the clinic is 00:00 and belongs to the day beginning', () => {
  // The hour h12 formatters render as "24", which would sort an
  // appointment to the end of a day it starts.
  const instant = new Date('2026-11-03T08:00:00.000Z')

  expect(zonedTime(instant, 'America/Tijuana')).toBe('00:00')
  expect(zonedDay(instant, 'America/Tijuana')).toBe('2026-11-03')
})

test('a day is spelled with its month in words', () => {
  // 3 November 2026 is a Tuesday; read off a calendar, not computed here.
  expect(spellDay('2026-11-03')).toBe('martes 3 de noviembre de 2026')
  expect(spellDay('2026-01-31')).toBe('sábado 31 de enero de 2026')
})

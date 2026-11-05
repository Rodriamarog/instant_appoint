import { cleanup, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, expect, test } from 'vitest'

import { Shell } from '@/components/shell'

/**
 * The seam is the rendered nav. What a signed-in person can reach by clicking
 * is the whole of this ticket's first criterion, and it is a property of this
 * component alone — the route table below it is tested in App.test.tsx.
 */
afterEach(cleanup)

function nav(path: string) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <Shell />
    </MemoryRouter>,
  )

  return screen.getByRole('navigation')
}

test('the signed-in navigation is Inicio, Asistente and Configuración, and nothing else', () => {
  const bar = nav('/inicio')

  expect(
    within(bar)
      .getAllByRole('link')
      .map((link) => link.textContent),
  ).toEqual(['Inicio', 'Asistente', 'Configuración'])
})

/**
 * Configuración holds four sub-pages, so the tab has to stay lit while
 * somebody is inside one. The styling was an exact pathname match, which
 * would have left both tabs looking unvisited on every page that matters.
 */
test('the Configuración tab stays current while inside one of its pages', () => {
  const bar = nav('/configuracion/plantillas')

  const current = within(bar).getByRole('link', { name: 'Configuración' })
  expect(current.getAttribute('aria-current')).toBe('page')
  expect(
    within(bar).getByRole('link', { name: 'Inicio' }).getAttribute('aria-current'),
  ).toBeNull()
})

/**
 * /inicio must not light up for /inicio-anything, which a bare startsWith
 * would do.
 */
test('a tab is not current for a path that merely starts with its own', () => {
  const bar = nav('/configuracion')

  expect(
    within(bar).getByRole('link', { name: 'Inicio' }).getAttribute('aria-current'),
  ).toBeNull()
})

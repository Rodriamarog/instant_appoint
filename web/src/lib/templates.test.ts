import { expect, test } from 'vitest'

import { placeholders, templateBody, validateDraft } from '@/lib/templates'

test('a repeated placeholder still takes one value', () => {
  // Meta fills positionally, so {{1}} appearing twice is one parameter, and
  // sending two would shift every later value by one.
  expect(placeholders('Hola {{1}}, {{1}} — tu cita es {{2}}.')).toEqual([1, 2])
})

test('a body with no placeholders takes no parameters', () => {
  expect(placeholders('Welcome aboard!')).toEqual([])
})

test('the body is the BODY component, whatever else Meta sent', () => {
  expect(
    templateBody({
      name: 'n',
      status: 'APPROVED',
      category: 'UTILITY',
      language: 'es',
      components: [
        { type: 'HEADER', text: 'Header' },
        { type: 'BODY', text: 'The body.' },
        { type: 'FOOTER', text: 'Footer' },
      ],
    }),
  ).toBe('The body.')
})

test('a draft is complete only with a name, category, language and body', () => {
  expect(
    validateDraft({ name: '', category: '', language: '', body: '' }),
  ).toEqual({
    name: 'A name is required.',
    category: 'Choose a category.',
    language: 'Choose a language.',
    body: 'A body is required.',
  })

  expect(
    validateDraft({
      name: 'appointment_reminder',
      category: 'UTILITY',
      language: 'es',
      body: 'Hola {{1}}.',
    }),
  ).toEqual({})
})

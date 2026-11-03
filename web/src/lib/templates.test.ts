import { expect, test } from 'vitest'

import {
  QUICK_REPLIES,
  placeholders,
  templateBody,
  templateButtons,
  validateDraft,
} from '@/lib/templates'

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
        {
          type: 'BUTTONS',
          buttons: [
            { type: 'QUICK_REPLY', text: 'Confirmar' },
            { type: 'QUICK_REPLY', text: 'Cancelar' },
          ],
        },
      ],
    }),
  ).toBe('The body.')
})

test('the buttons are the labels of the BUTTONS component, in order', () => {
  // Order is the whole identity of a quick reply: Meta addresses one by its
  // index under the message, not by its text.
  expect(
    templateButtons({
      name: 'appointment_reminder',
      status: 'APPROVED',
      category: 'UTILITY',
      language: 'es',
      components: [
        { type: 'BODY', text: 'Hola {{1}}.' },
        {
          type: 'BUTTONS',
          buttons: [
            { type: 'QUICK_REPLY', text: 'Confirmar' },
            { type: 'QUICK_REPLY', text: 'Cancelar' },
          ],
        },
      ],
    }),
  ).toEqual(['Confirmar', 'Cancelar'])
})

test('a template Meta sent without buttons has none', () => {
  // Every template created before buttons existed looks like this, and the
  // reminder settings dialog still has to be able to choose one.
  expect(
    templateButtons({
      name: 'welcome',
      status: 'APPROVED',
      category: 'MARKETING',
      language: 'en_US',
      components: [{ type: 'BODY', text: 'Welcome aboard!' }],
    }),
  ).toEqual([])
})

test('the suggested button labels are written in the template’s own language', () => {
  // A Spanish-speaking patient tapping "Confirm" is the whole failure this
  // prevents: the buttons live inside the template, so they are approved in
  // one language and cannot be swapped per recipient afterwards.
  expect(QUICK_REPLIES.es).toEqual(['Confirmar', 'Cancelar'])
  expect(QUICK_REPLIES.en_US).toEqual(['Confirm', 'Cancel'])
  expect(QUICK_REPLIES.pt_BR).toEqual(['Confirmar', 'Cancelar'])
})

test('a draft is complete only with a name, category, language and body', () => {
  expect(
    validateDraft({
      name: '',
      category: '',
      language: '',
      body: '',
      buttons: [],
    }),
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
      buttons: ['Confirmar', 'Cancelar'],
    }),
  ).toEqual({})
})

test('a button Meta would refuse is caught before the review clock starts', () => {
  const draft = {
    name: 'appointment_reminder',
    category: 'UTILITY',
    language: 'es',
    body: 'Hola {{1}}.',
  }

  expect(validateDraft({ ...draft, buttons: ['Confirmar', '  '] })).toEqual({
    buttons: 'Cada botón necesita un texto.',
  })

  expect(
    validateDraft({
      ...draft,
      buttons: ['Confirmar', 'Cancelar esta cita de inmediato'],
    }),
  ).toEqual({
    buttons: 'El texto de un botón no puede pasar de 25 caracteres.',
  })

  expect(
    validateDraft({
      ...draft,
      buttons: ['Confirmar', 'Cancelar', 'Reagendar', 'Llamar'],
    }),
  ).toEqual({
    buttons: 'Un mensaje admite como máximo tres botones.',
  })
})

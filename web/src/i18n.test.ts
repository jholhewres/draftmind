import { catalogs, missingKeys, pickLocale, translate } from './i18n'

test('pt-BR has every key of the en catalog', () => {
  expect(missingKeys(catalogs.en, catalogs['pt-BR'])).toEqual([])
})

test('a key present in en but missing in pt-BR is reported', () => {
  expect(missingKeys({ a: 'A', b: 'B' }, { a: 'A' })).toEqual(['b'])
})

test('a Portuguese browser picks pt-BR', () => {
  expect(pickLocale(['pt-BR', 'en-US'])).toBe('pt-BR')
  expect(pickLocale(['pt'])).toBe('pt-BR')
  expect(pickLocale(['fr-FR', 'PT-br'])).toBe('pt-BR')
})

test('an en-* browser gets English and any other language gets pt-BR', () => {
  expect(pickLocale(['en-GB'])).toBe('en')
  expect(pickLocale(['fr-FR', 'de'])).toBe('pt-BR')
  expect(pickLocale([])).toBe('pt-BR')
})

test('translate fills the variables of a message', () => {
  expect(translate('pt-BR', 'heroes.noMatch', { query: 'zzz' })).toBe('Nenhum herói encontrado para “zzz”.')
  expect(translate('en', 'app.sync.status', { patch: '7.41', ago: '3 hours ago' })).toBe('Patch 7.41 · updated 3 hours ago')
})

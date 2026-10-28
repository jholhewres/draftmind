import { expect, test } from './fixtures.ts'

// Anonymous groups: each test is named by its criterion alone.
test.describe(() => {
  test.use({ locale: 'pt-BR' })

  test("With the browser in pt-BR, the hero grid's search label, patch label and empty state render in Portuguese", async ({
    page,
    escaped,
  }) => {
    await page.goto('/#/heroes')

    await expect(page.getByRole('list', { name: 'Heróis' })).toBeVisible()
    await expect(page.getByText('Patch 7.41')).toBeVisible()
    await page.getByRole('searchbox', { name: 'Buscar heróis' }).fill('zzz')
    await expect(page.getByText('Nenhum herói encontrado para “zzz”.')).toBeVisible()
    await expect(page.getByText(/Dota 2 é marca registrada da Valve/)).toBeVisible()
    expect(escaped).toEqual([])
    await expect(page).toHaveScreenshot('pt-br-empty-state.png', { fullPage: true })
  })
})

test.describe(() => {
  test.use({ locale: 'fr-FR' })

  test('An unsupported browser language falls back to English', async ({ page, escaped }) => {
    await page.goto('/#/heroes')

    await expect(page.getByRole('list', { name: 'Heroes' })).toBeVisible()
    await page.getByRole('searchbox', { name: 'Search heroes' }).fill('zzz')
    await expect(page.getByText('No hero matches “zzz”.')).toBeVisible()
    await expect(page.getByText(/Dota 2 is a Valve trademark/)).toBeVisible()
    expect(escaped).toEqual([])
    await expect(page).toHaveScreenshot('fallback-en-empty-state.png', { fullPage: true })
  })
})

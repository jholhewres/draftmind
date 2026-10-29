import { expect, test } from './fixtures.ts'

test('GET /api/sync/status and the footer show the current patch and last sync time', async ({ page, escaped }) => {
  await page.goto('/#/heroes')

  const footer = page.getByRole('contentinfo')
  await expect(footer.getByText('Patch 7.41 · updated 3 hours ago')).toBeVisible()
  await expect(footer.getByText(/Dota 2 is a Valve trademark/)).toBeVisible()
  expect(escaped).toEqual([])
  await footer.scrollIntoViewIfNeeded()
  await expect(page).toHaveScreenshot('footer-sync-status.png')
})

test.describe(() => {
  test.use({ locale: 'pt-BR' })

  test('The footer says when the data was synced, in Portuguese', async ({ page, escaped }) => {
    await page.goto('/#/heroes')

    const footer = page.getByRole('contentinfo')
    await expect(footer.getByText('Patch 7.41 · atualizado há 3 horas')).toBeVisible()
    expect(escaped).toEqual([])
    await footer.scrollIntoViewIfNeeded()
    await expect(page).toHaveScreenshot('footer-sync-status-pt-br.png')
  })
})

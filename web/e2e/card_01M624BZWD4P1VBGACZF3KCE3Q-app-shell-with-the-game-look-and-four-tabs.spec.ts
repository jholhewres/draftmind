import { expect, imagesLoaded, portraitsLoaded, test } from './fixtures.ts'
import heroes from './fixtures/heroes.json' with { type: 'json' }

const tabs = ['Draft', 'Coach', 'Heroes', 'Patches']
const note = 'Not affiliated with or endorsed by Valve. Dota 2 is a Valve trademark. Never reads or changes the game client.'

test('The tab bar switches between Draft, Coach, Heroes and Patches and marks the current tab with aria-current', async ({
  page,
  escaped,
}) => {
  await page.goto('/')
  const nav = page.getByRole('navigation', { name: 'Main' })
  await expect(nav.getByRole('link')).toHaveText(tabs)
  await expect(nav.getByRole('link', { name: 'Draft' })).toHaveAttribute('aria-current', 'page')

  for (const tab of tabs) {
    await nav.getByRole('link', { name: tab }).click()
    await expect(page).toHaveURL(new RegExp(`#/${tab.toLowerCase()}$`))
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(tab)
    await expect(nav.locator('[aria-current="page"]')).toHaveText(tab)
  }
  await expect(page.getByText('Coming soon.')).toBeVisible()
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('shell-patches.png')
})

test("The Heroes tab shows each hero's portrait from its image path and filters by primary attribute", async ({
  page,
  escaped,
}) => {
  await page.goto('/#/heroes')
  const grid = page.getByRole('list', { name: 'Heroes' })
  await expect(grid.getByRole('listitem')).toHaveCount(heroes.heroes.length)
  const srcs = await grid.locator('img').evaluateAll((els) => els.map((el) => el.getAttribute('src')))
  expect(srcs).toEqual(heroes.heroes.map((h) => `https://cdn.cloudflare.steamstatic.com${h.img}`))

  const filter = page.getByRole('group', { name: 'Filter by attribute' })
  for (const [label, attr] of [
    ['Strength', 'str'],
    ['Agility', 'agi'],
    ['Intelligence', 'int'],
    ['Universal', 'all'],
  ]) {
    await filter.getByRole('button', { name: label }).click()
    await expect(filter.getByRole('button', { name: label })).toHaveAttribute('aria-pressed', 'true')
    await expect(grid.getByRole('listitem')).toHaveText(heroes.heroes.filter((h) => h.primaryAttr === attr).map((h) => h.name))
  }
  await filter.getByRole('button', { name: 'Strength' }).click()
  await portraitsLoaded(page)
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('hero-grid-strength.png')
})

test('The trademark and fair-play note renders on every main tab', async ({ page, escaped }) => {
  // Ends on Coach, the one screen no other baseline covers.
  for (const tab of ['Draft', 'Heroes', 'Patches', 'Coach']) {
    await page.goto(`/#/${tab.toLowerCase()}`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(tab)
    await expect(page.getByRole('contentinfo').getByText(note)).toBeVisible()
  }
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('note-coach.png')
})

test('Visual tests match the shell and hero grid baselines at 390x844 (bottom tab bar) and 1280x800 (left rail)', async ({
  page,
  escaped,
}, testInfo) => {
  await page.goto('/#/heroes')
  await expect(page.getByRole('list', { name: 'Heroes' }).getByRole('listitem')).toHaveCount(heroes.heroes.length)
  await portraitsLoaded(page)

  const viewport = page.viewportSize()!
  const bar = (await page.getByRole('navigation', { name: 'Main' }).boundingBox())!
  if (testInfo.project.name === 'phone') {
    // Bottom tab bar, 64px tall, full width.
    expect(bar).toEqual({ x: 0, y: viewport.height - 64, width: viewport.width, height: 64 })
  } else {
    // Left rail, full height; the content column stays within 1200px.
    expect(bar).toMatchObject({ x: 0, y: 0, height: viewport.height })
    const column = (await page.locator('.column').boundingBox())!
    expect(column.x).toBeGreaterThanOrEqual(bar.width)
    expect(column.width).toBeLessThanOrEqual(1200)
  }
  expect(escaped).toEqual([])
  await expect(page).toHaveScreenshot('shell-heroes.png')
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Draft')
  // The Draft tab ranks the empty draft; its portraits come from the fixture.
  await expect(page.getByRole('list', { name: 'Suggestions' }).getByRole('listitem')).toHaveCount(5)
  await imagesLoaded(page.locator('main'))
  await expect(page).toHaveScreenshot('shell-draft.png')
})

import { fireEvent, render, screen } from '@testing-library/react'
import ItemCard from './ItemCard'

const bkb = { id: 116, key: 'black_king_bar', name: 'Black King Bar', cost: 4050, img: '/bkb.png' }

test('an item card shows its CDN image, and the initial once the image fails', () => {
  const { container } = render(<ItemCard item={bkb} />)
  const img = container.querySelector('img')
  expect(img).toHaveAttribute('src', expect.stringMatching(/^https:\/\/[^/]+\/bkb\.png$/))
  expect(screen.getByText('4,050 gold')).toBeInTheDocument()
  fireEvent.error(img!)
  expect(container.querySelector('img')).toBeNull()
  expect(container.querySelector('.item-fallback')).toHaveTextContent('B')
})

test('an image off the CDN host, or none, falls back to the initial', () => {
  for (const img of ['//evil.example/x.png', 'https://evil.example/x.png', undefined]) {
    const { container, unmount } = render(<ItemCard item={{ ...bkb, img, cost: 0 }} />)
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('.item-fallback')).toHaveTextContent('B')
    expect(screen.getByText('No cost')).toBeInTheDocument()
    unmount()
  }
})

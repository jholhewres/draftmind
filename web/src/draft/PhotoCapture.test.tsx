import { fireEvent, render, screen } from '@testing-library/react'
import { vi } from 'vitest'
import PhotoCapture from './PhotoCapture'

test('the photo screen opens the rear camera, the gallery, or the board by hand', () => {
  const onPhoto = vi.fn()
  const { container } = render(<PhotoCapture onPhoto={onPhoto} />)
  expect(screen.getByRole('heading', { level: 1, name: 'Photograph the pick' })).toBeInTheDocument()
  const [camera, gallery] = container.querySelectorAll('input[type=file]')
  expect(camera).toHaveAttribute('accept', 'image/*')
  expect(camera).toHaveAttribute('capture', 'environment')
  expect(gallery).toHaveAttribute('accept', 'image/*')
  expect(gallery).not.toHaveAttribute('capture')
  expect(screen.getByText('Tap to photograph').closest('label')).toContainElement(camera as HTMLElement)
  expect(screen.getByText('From the gallery').closest('label')).toContainElement(gallery as HTMLElement)
  expect(screen.getByRole('link', { name: 'Build by hand' })).toHaveAttribute('href', '#/draft/manual')
  expect(screen.getByText('The photo is read and dropped, never stored.')).toBeInTheDocument()

  const photo = new File(['x'], 'pick.jpg', { type: 'image/jpeg' })
  fireEvent.change(gallery, { target: { files: [photo] } })
  expect(onPhoto).toHaveBeenCalledWith(photo)
})

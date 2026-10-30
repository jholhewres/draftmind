import { fireEvent, render, screen } from '@testing-library/react'
import { vi } from 'vitest'
import ModePicker from './ModePicker'

test('the mode picker offers the supported modes, lists the rest disabled, and reports a change', () => {
  const onChange = vi.fn()
  render(<ModePicker mode="all_pick_ranked" onChange={onChange} />)
  const select = screen.getByRole('combobox', { name: 'Game mode' })
  expect(select).toHaveValue('all_pick_ranked')
  const options = screen.getAllByRole('option') as HTMLOptionElement[]
  expect(options).toHaveLength(10)
  expect(options.filter((o) => o.disabled).map((o) => o.value)).toEqual(['ability_draft', 'custom'])
  expect(screen.getByText(/Ability Draft and custom games are not supported/)).toBeInTheDocument()
  fireEvent.change(select, { target: { value: 'captains_mode' } })
  expect(onChange).toHaveBeenCalledWith('captains_mode')
})

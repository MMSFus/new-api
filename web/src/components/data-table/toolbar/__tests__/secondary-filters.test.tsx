/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  getCoreRowModel,
  useReactTable,
  type ColumnFiltersState,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { Input } from '@/components/ui/input'

import { DataTableToolbar } from '../toolbar'

const rows = [
  { name: 'Channel', model: 'model', status: 'enabled', group: 'a' },
]
const filters = [
  {
    columnId: 'status',
    title: 'Status',
    options: [{ value: 'enabled', label: 'Enabled' }],
  },
  {
    columnId: 'group',
    title: 'Group',
    options: [{ value: 'vip', label: 'VIP' }],
    secondary: true,
  },
]

function Fixture(props: {
  collapsibleOnMobile?: boolean
  initialFilters?: ColumnFiltersState
}) {
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(
    props.initialFilters ?? []
  )
  const table = useReactTable({
    data: rows,
    columns: [
      { accessorKey: 'name' },
      { accessorKey: 'model' },
      { accessorKey: 'status' },
      { accessorKey: 'group' },
    ],
    getCoreRowModel: getCoreRowModel(),
    state: { columnFilters },
    onColumnFiltersChange: setColumnFilters,
  })
  return (
    <DataTableToolbar
      table={table}
      collapsibleOnMobile={props.collapsibleOnMobile}
      searchPlaceholder='Search channels'
      filters={filters}
      secondarySearch={
        <Input
          aria-label='Model'
          value={String(table.getColumn('model')?.getFilterValue() ?? '')}
          onChange={(event) =>
            table.getColumn('model')?.setFilterValue(event.target.value)
          }
        />
      }
    />
  )
}

function setMobileViewport(mobile: boolean) {
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...original(query),
    matches: mobile && query === '(max-width: 640px)',
  }))
}

afterEach(() => {
  vi.restoreAllMocks()
})

it('hides secondary filters on desktop until More filters is opened', async () => {
  setMobileViewport(false)
  const user = userEvent.setup()
  render(<Fixture />)

  expect(screen.getByRole('button', { name: /^Status/ })).toBeVisible()
  expect(
    screen.queryByRole('button', { name: /^Group/ })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('textbox', { name: 'Model' })
  ).not.toBeInTheDocument()

  const toggle = screen.getByRole('button', { name: 'More filters' })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  toggle.focus()
  await user.keyboard('{Enter}')

  expect(screen.getByRole('button', { name: 'Fewer filters' })).toHaveAttribute(
    'aria-expanded',
    'true'
  )
  expect(screen.getByRole('button', { name: /^Group/ })).toBeVisible()
  expect(screen.getByRole('textbox', { name: 'Model' })).toBeVisible()
})

it('keeps secondary values after collapsing and resets them while hidden', async () => {
  setMobileViewport(false)
  const user = userEvent.setup()
  render(<Fixture initialFilters={[{ id: 'group', value: ['vip'] }]} />)

  await user.click(screen.getByRole('button', { name: 'More filters' }))
  expect(screen.getByRole('button', { name: /^Group/ })).toHaveTextContent(
    'VIP'
  )
  await user.type(screen.getByRole('textbox', { name: 'Model' }), 'gpt')
  await user.click(screen.getByRole('button', { name: 'Fewer filters' }))
  await user.click(screen.getByRole('button', { name: 'Reset' }))
  await user.click(screen.getByRole('button', { name: 'More filters' }))

  expect(screen.getByRole('textbox', { name: 'Model' })).toHaveValue('')
  expect(screen.getByRole('button', { name: 'Group' })).toBeVisible()
})

it('shows secondary filters inline in the collapsible mobile panel', () => {
  setMobileViewport(true)
  render(<Fixture collapsibleOnMobile />)

  expect(screen.getByRole('button', { name: /^Group/ })).toBeVisible()
  expect(screen.getByRole('textbox', { name: 'Model' })).toBeVisible()
  expect(
    screen.queryByRole('button', { name: /^(More|Fewer) filters$/ })
  ).not.toBeInTheDocument()
})

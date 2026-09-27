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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toaster, toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { RedemptionsPrimaryButtons } from '../redemptions-primary-buttons'
import { RedemptionsProvider } from '../redemptions-provider'

afterEach(() => {
  toast.dismiss()
  vi.restoreAllMocks()
})

test('delete invalid lives in the more-actions menu and still confirms before deleting', async () => {
  const remove = vi
    .spyOn(api, 'delete')
    .mockResolvedValue({ data: { success: true, data: 2 } })
  const user = userEvent.setup()
  render(
    <RedemptionsProvider>
      <RedemptionsPrimaryButtons />
      <Toaster />
    </RedemptionsProvider>
  )

  expect(
    screen.queryByRole('button', { name: 'Delete Invalid' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Create Code' })).toBeVisible()

  await user.click(screen.getByRole('button', { name: 'More actions' }))
  await user.click(
    await screen.findByRole('menuitem', { name: 'Delete Invalid' })
  )
  const dialog = await screen.findByRole('alertdialog')
  expect(remove).not.toHaveBeenCalled()
  await user.click(
    within(dialog).getByRole('button', { name: 'Delete Invalid' })
  )

  await waitFor(() =>
    expect(remove).toHaveBeenCalledWith('/api/redemption/invalid')
  )
})

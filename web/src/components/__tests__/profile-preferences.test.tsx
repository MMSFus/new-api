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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { ProfileDropdown } from '@/components/profile-dropdown'
import { SidebarProvider } from '@/components/ui/sidebar'
import { DirectionProvider } from '@/context/direction-provider'
import { LayoutProvider } from '@/context/layout-provider'
import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { statusQueryOptions } from '@/lib/status-query'

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => vi.fn(),
}))

afterEach(() => {
  window.localStorage.clear()
})

function renderMenu(showPreferences?: boolean) {
  const queryClient = new QueryClient()
  queryClient.setQueryData(statusQueryOptions.queryKey, {})
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <ThemeCustomizationProvider>
          <DirectionProvider>
            <LayoutProvider>
              <SidebarProvider>
                <ProfileDropdown showPreferences={showPreferences} />
              </SidebarProvider>
            </LayoutProvider>
          </DirectionProvider>
        </ThemeCustomizationProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

test('theme settings entry in the profile menu opens the theme drawer', async () => {
  const user = userEvent.setup()
  renderMenu()

  await user.click(screen.getByRole('button'))
  await user.click(
    await screen.findByRole('menuitem', { name: 'Theme Settings' })
  )

  expect(
    await screen.findByRole('dialog', { name: 'Theme Settings' })
  ).toBeVisible()
})

test('profile menu offers the language picker next to theme settings', async () => {
  const user = userEvent.setup()
  renderMenu()

  await user.click(screen.getByRole('button'))

  expect(
    await screen.findByRole('menuitem', { name: /Change language/ })
  ).toBeVisible()
})

test('profile menu hides preferences when the header turns them off', async () => {
  const user = userEvent.setup()
  renderMenu(false)

  await user.click(screen.getByRole('button'))
  await screen.findByRole('menuitem', { name: 'Profile' })

  expect(screen.queryByRole('menuitem', { name: 'Theme Settings' })).toBeNull()
})

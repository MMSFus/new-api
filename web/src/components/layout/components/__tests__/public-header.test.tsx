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
import { render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'

import type { TopNavLink } from '@/components/layout/types'

import { PublicHeader } from '../public-header'

const mocks = vi.hoisted(() => ({
  user: null as { username: string } | null,
  links: [] as TopNavLink[],
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children: React.ReactNode; to?: string }) => (
    <a href={props.to}>{props.children}</a>
  ),
  useNavigate: () => vi.fn(),
  useRouterState: () => ({ location: { pathname: '/' } }),
}))

vi.mock('@/stores/auth-store', () => ({
  useAuthStore: () => ({ auth: { user: mocks.user } }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'Huidian',
    logo: '',
    loading: false,
    logoLoaded: true,
  }),
}))

vi.mock('@/hooks/use-top-nav-links', () => ({
  useTopNavLinks: () => mocks.links,
}))

vi.mock('@/hooks/use-notifications', () => ({
  useNotifications: () => ({
    popoverOpen: false,
    setPopoverOpen: vi.fn(),
    unreadCount: 0,
    activeTab: 'notice',
    setActiveTab: vi.fn(),
    notice: '',
    announcements: [],
    loading: false,
  }),
}))

vi.mock('@/components/notification-popover', () => ({
  NotificationPopover: () => <button type='button'>Notifications</button>,
}))

vi.mock('@/features/system-update/system-update-action', () => ({
  SystemUpdateAction: () => null,
}))

vi.mock('@/components/language-switcher', () => ({
  LanguageSwitcher: () => <button type='button'>Change language</button>,
}))

vi.mock('@/components/theme-switch', () => ({
  ThemeSwitch: () => <button type='button'>Toggle theme</button>,
}))

vi.mock('@/components/profile-dropdown', () => ({
  ProfileDropdown: (props: { showQuickPreferences?: boolean }) => (
    <button
      type='button'
      data-quick-preferences={String(props.showQuickPreferences === true)}
    >
      Account menu
    </button>
  ),
}))

beforeEach(() => {
  mocks.user = null
  mocks.links = [
    { title: 'Home', href: '/' },
    { title: 'Model Square', href: '/pricing', requiresAuth: true },
  ]
})

it('hides sign-in-only links from signed-out visitors', () => {
  render(<PublicHeader />)

  expect(screen.getAllByRole('link', { name: 'Home' }).length).toBeGreaterThan(
    0
  )
  expect(screen.queryByRole('link', { name: 'Model Square' })).toBeNull()
})

it('keeps standalone language and theme switchers for signed-out visitors', () => {
  render(<PublicHeader />)

  expect(screen.getByRole('button', { name: 'Change language' })).toBeVisible()
  expect(
    screen.getAllByRole('button', { name: 'Toggle theme' }).length
  ).toBeGreaterThan(0)
  expect(screen.queryByRole('button', { name: 'Account menu' })).toBeNull()
})

it('folds language and theme into the avatar menu once signed in', () => {
  mocks.user = { username: 'alice' }
  mocks.links = [{ title: 'Model Square', href: '/pricing' }]
  render(<PublicHeader />)

  expect(screen.queryByRole('button', { name: 'Change language' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Toggle theme' })).toBeNull()
  const menus = screen.getAllByRole('button', { name: 'Account menu' })
  expect(menus.length).toBeGreaterThan(0)
  for (const menu of menus) {
    expect(menu).toHaveAttribute('data-quick-preferences', 'true')
  }
  expect(
    screen.getAllByRole('link', { name: 'Model Square' }).length
  ).toBeGreaterThan(0)
})

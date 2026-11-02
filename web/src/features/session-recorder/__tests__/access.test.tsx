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
import { screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { allowedTabs, sessionRecorderAccess } from '../lib'
import { FULL_GRANTS, mockApi, NOW, renderPage, setUser } from './harness'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

const savedAuth = useAuthStore.getState().auth

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW * 1000)
})

afterEach(() => {
  useAuthStore.setState({ auth: savedAuth })
  vi.restoreAllMocks()
})

function tabNames() {
  const list = screen.getByRole('tablist', {
    name: 'Session recorder sections',
  })
  return within(list)
    .getAllByRole('tab')
    .map((tab) => tab.textContent)
}

describe('tabs follow the two read grants', () => {
  test('an admin with only the revenue grant sees only Revenue details', async () => {
    setUser(ROLE.ADMIN, { session_recorder_revenue: { read: true } })
    mockApi()
    await renderPage('/session-recorder?tab=content')

    expect(await screen.findByText('Compressed data')).toBeVisible()
    expect(tabNames()).toEqual(['Revenue details'])
    expect(
      screen.queryByRole('columnheader', { name: 'Session ID' })
    ).not.toBeInTheDocument()
  })

  test('an admin with only the content grant sees only Conversation review and never loads revenue', async () => {
    setUser(ROLE.ADMIN, { session_recorder_content: { read: true } })
    const api = mockApi()
    await renderPage('/session-recorder')

    expect(await screen.findByRole('button', { name: 'sess-1' })).toBeVisible()
    expect(tabNames()).toEqual(['Conversation review'])
    expect(api.params('/api/session_recorder/revenue/summary')).toHaveLength(0)
  })

  test('an admin with both grants sees both tabs in order', async () => {
    setUser(ROLE.ADMIN, FULL_GRANTS)
    mockApi()
    await renderPage('/session-recorder')

    await screen.findByText('Compressed data')
    expect(tabNames()).toEqual(['Revenue details', 'Conversation review'])
  })

  test('revenue read without write hides the rule editor', async () => {
    setUser(ROLE.ADMIN, { session_recorder_revenue: { read: true } })
    mockApi()
    await renderPage('/session-recorder')

    await screen.findByText('Compressed data')
    expect(
      screen.queryByRole('button', { name: 'Edit rules' })
    ).not.toBeInTheDocument()
  })
})

describe('page access', () => {
  test('an admin without either grant is not allowed on the page', () => {
    const access = sessionRecorderAccess({
      id: 2,
      username: 'a',
      role: ROLE.ADMIN,
    })
    expect(allowedTabs(access)).toEqual([])
  })

  test('root holds every section without explicit grants', () => {
    const access = sessionRecorderAccess({
      id: 1,
      username: 'r',
      role: ROLE.SUPER_ADMIN,
    })
    expect(access).toEqual({
      revenue: true,
      revenueWrite: true,
      content: true,
      backfill: true,
    })
  })
})

describe('historical backfill entry', () => {
  test('root sees the backfill entry in the page header', async () => {
    setUser(ROLE.SUPER_ADMIN)
    mockApi()
    await renderPage('/session-recorder')

    expect(
      await screen.findByRole('button', { name: /Historical index backfill/ })
    ).toBeVisible()
  })

  test('an admin with every grant gets no backfill entry and no status request', async () => {
    setUser(ROLE.ADMIN, FULL_GRANTS)
    const api = mockApi()
    await renderPage('/session-recorder')

    await screen.findByText('Compressed data')
    expect(
      screen.queryByRole('button', { name: /Historical index backfill/ })
    ).not.toBeInTheDocument()
    expect(api.params('/api/session_recorder/backfill')).toHaveLength(0)
  })
})

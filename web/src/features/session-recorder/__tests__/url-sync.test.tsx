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
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { FULL_GRANTS, mockApi, NOW, renderPage, setUser } from './harness'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: vi.fn() },
}))

const DAY = 24 * 3600
const SESSIONS = '/api/session_recorder/sessions'
const SUMMARY = '/api/session_recorder/revenue/summary'
const savedAuth = useAuthStore.getState().auth

beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(NOW * 1000)
  setUser(ROLE.ADMIN, FULL_GRANTS)
})

afterEach(() => {
  useAuthStore.setState({ auth: savedAuth })
  vi.restoreAllMocks()
})

test('URL filters of the content tab seed the session query and the inputs', async () => {
  const api = mockApi()
  await renderPage(
    '/session-recorder?tab=content&model=claude&user=7&session=sess-1&range=24h&page=2'
  )

  await screen.findByRole('button', { name: 'sess-1' })
  expect(api.params(SESSIONS)[0]).toEqual({
    start_timestamp: NOW - DAY,
    end_timestamp: NOW,
    model: 'claude',
    user_id: 7,
    session_id: 'sess-1',
    p: 2,
    page_size: 20,
  })
  expect(screen.getByRole('textbox', { name: 'User ID' })).toHaveValue('7')
  expect(screen.getByRole('textbox', { name: 'Session ID' })).toHaveValue(
    'sess-1'
  )
})

test('typing a session filter writes it to the URL and returns to the first page', async () => {
  mockApi()
  const { router } = await renderPage('/session-recorder?tab=content&page=3')
  await screen.findByRole('button', { name: 'sess-1' })

  await userEvent
    .setup()
    .type(screen.getByRole('textbox', { name: 'User ID' }), '42')

  await waitFor(() =>
    expect(router.state.location.search).toEqual({ tab: 'content', user: 42 })
  )
})

test('revenue range and model come from the URL and pick the hourly granularity', async () => {
  const api = mockApi()
  await renderPage('/session-recorder?model=gpt&range=24h')

  await screen.findByText('Compressed data')
  await waitFor(() =>
    expect(api.params(SUMMARY)[0]).toEqual({
      start_timestamp: NOW - DAY,
      end_timestamp: NOW,
      model: 'gpt',
      granularity: 'hour',
    })
  )
})

test('switching tabs keeps the shared filters in the URL', async () => {
  mockApi()
  const { router } = await renderPage('/session-recorder?model=gpt&range=30d')
  await screen.findByText('Compressed data')

  await userEvent
    .setup()
    .click(screen.getByRole('tab', { name: 'Conversation review' }))

  await waitFor(() =>
    expect(router.state.location.search).toEqual({
      model: 'gpt',
      range: '30d',
      tab: 'content',
    })
  )
  expect(screen.getByRole('textbox', { name: 'Model Name' })).toHaveValue('gpt')
})

test('the breakdown dimension is restored from the URL', async () => {
  mockApi()
  await renderPage('/session-recorder?breakdown=model')

  const tabs = await screen.findByRole('tablist', {
    name: 'Breakdown dimension',
  })
  expect(within(tabs).getByRole('tab', { name: 'By model' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(await screen.findByRole('button', { name: 'gpt-5' })).toBeVisible()
})

test('reset clears the filters but stays on the current tab', async () => {
  mockApi()
  const { router } = await renderPage(
    '/session-recorder?tab=content&user=7&model=x'
  )
  await screen.findByRole('button', { name: 'sess-1' })

  await userEvent.setup().click(screen.getByRole('button', { name: 'Reset' }))

  await waitFor(() =>
    expect(router.state.location.search).toEqual({ tab: 'content' })
  )
})

test('a revenue range over 92 days shows the range error instead of querying', async () => {
  const api = mockApi()
  await renderPage(`/session-recorder?start=${NOW - 100 * DAY}&end=${NOW}`)

  expect(
    await screen.findByText('Time range cannot exceed 92 days')
  ).toBeVisible()
  expect(api.params(SUMMARY)).toHaveLength(0)
})

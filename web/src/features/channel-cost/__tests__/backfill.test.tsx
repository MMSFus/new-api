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
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { ChannelCostFilters, CostBackfillTask } from '../api'
import { ChannelCostBackfillPanel } from '../backfill-panel'

const filters: ChannelCostFilters = {
  start_timestamp: 100,
  end_timestamp: 200,
  channel: 7,
  model_name: 'selected',
  username: 'user',
  group: 'vip',
}
const task: CostBackfillTask = {
  id: 1,
  task_id: 'backfill',
  type: 'channel_cost_backfill',
  status: 'running',
  created_at: 100,
  updated_at: 100,
  payload: {
    filter: {
      StartTimestamp: 300,
      EndTimestamp: 400,
      Channel: 9,
      ModelName: 'active-model',
      Username: 'active-user',
      Group: 'active-group',
    },
  },
  state: {
    total: 10,
    processed: 5,
    updated: 3,
    skipped: 2,
    progress: 50,
    reasons: { 'Invalid historical billing data': 2 },
  },
}
let client: QueryClient
const savedAuth = useAuthStore.getState().auth

beforeEach(() => {
  useAuthStore.setState({
    auth: {
      ...savedAuth,
      user: { id: 1, username: 'root', role: ROLE.SUPER_ADMIN },
    },
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
})
afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState({ auth: savedAuth })
  vi.restoreAllMocks()
})
function panel(current = filters, missing = 26315, enabled = true) {
  return (
    <QueryClientProvider client={client}>
      <ChannelCostBackfillPanel
        filters={current}
        missing={missing}
        enabled={enabled}
      />
    </QueryClientProvider>
  )
}
function mockStatus(active: CostBackfillTask | null = null, supported = true) {
  return vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { supported, task: active } },
  })
}

it('changing the period does not start a task; cancel does not post; confirmation submits its captured filters', async () => {
  mockStatus()
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { task, created: true } },
  })
  const view = render(panel())
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Backfill historical costs' })
    ).toBeEnabled()
  )
  view.rerender(panel({ ...filters, start_timestamp: 120 }))
  expect(post).not.toHaveBeenCalled()
  await userEvent.click(
    screen.getByRole('button', { name: 'Backfill historical costs' })
  )
  let dialog = screen.getByRole('alertdialog')
  expect(within(dialog).getByText(/26,315/)).toBeVisible()
  await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  expect(post).not.toHaveBeenCalled()
  await userEvent.click(
    screen.getByRole('button', { name: 'Backfill historical costs' })
  )
  dialog = screen.getByRole('alertdialog')
  view.rerender(panel({ ...filters, channel: 8 }))
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Start background backfill' })
  )
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/channel_cost/backfill', null, {
      params: { ...filters, start_timestamp: 120 },
    })
  )
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(
    screen.getByRole('button', { name: 'Backfill historical costs' })
  ).toBeDisabled()
})

it('an active task shows its own filters, progress and skip reasons while preventing another start', async () => {
  mockStatus(task)
  render(panel())
  expect(await screen.findByText('Backfilling historical costs')).toBeVisible()
  expect(screen.getByText(/active-model/)).toHaveTextContent('Channel #9')
  expect(screen.getByText(/active-model/)).toHaveTextContent('active-user')
  expect(screen.getByRole('progressbar', { name: 'Progress' })).toHaveAttribute(
    'aria-valuenow',
    '50'
  )
  expect(screen.getByText('Invalid historical billing data: 2')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Backfill historical costs' })
  ).toBeDisabled()
})

it('a failed submission keeps confirmation open and allows retry after displaying the API error', async () => {
  mockStatus()
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: false, message: 'Backfill unavailable' },
  })
  render(panel())
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Backfill historical costs' })
    ).toBeEnabled()
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Backfill historical costs' })
  )
  const dialog = screen.getByRole('alertdialog')
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Start background backfill' })
  )
  expect(await within(dialog).findByRole('alert')).toHaveTextContent(
    'Backfill unavailable'
  )
  expect(
    within(dialog).getByRole('button', { name: 'Start background backfill' })
  ).toBeEnabled()
  post.mockResolvedValue({
    data: { success: true, data: { task, created: false } },
  })
  await userEvent.click(
    within(dialog).getByRole('button', { name: 'Start background backfill' })
  )
  expect(
    await screen.findByText(
      'A backfill task is already active. Showing its range.'
    )
  ).toBeVisible()
})

it('status failure prevents starting and the visible retry reloads status', async () => {
  const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('offline'))
  render(panel())
  expect(
    await screen.findByText('Failed to load backfill status')
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Backfill historical costs' })
  ).toBeDisabled()
  get.mockResolvedValue({
    data: { success: true, data: { supported: true, task: null } },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Backfill historical costs' })
    ).toBeEnabled()
  )
})

it('completion refreshes the report and preserves failure progress for a later retry', async () => {
  mockStatus(task)
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  render(panel())
  await screen.findByText('Backfilling historical costs')
  act(() =>
    client.setQueryData(['channel-cost-backfill'], {
      supported: true,
      task: { ...task, status: 'failed', result: task.state },
    })
  )
  expect(
    await screen.findByText('Historical cost backfill incomplete')
  ).toBeVisible()
  expect(
    screen.getByText(
      'Completed estimates are preserved. Retry processes records that are still uncosted.'
    )
  ).toBeVisible()
  await waitFor(() =>
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ['channel-cost'] })
  )
  expect(
    screen.getByRole('button', { name: 'Backfill historical costs' })
  ).toBeEnabled()
})

it('unsupported storage disables backfill and a zero missing count hides the action', async () => {
  mockStatus(null, false)
  const view = render(panel())
  expect(
    await screen.findByText(
      'Historical cost backfill is not supported by this log database'
    )
  ).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Backfill historical costs' })
  ).toBeDisabled()
  view.rerender(panel(filters, 0))
  expect(
    screen.queryByRole('button', { name: 'Backfill historical costs' })
  ).not.toBeInTheDocument()
})

it('non-root administrators see no backfill controls and do not request task status', () => {
  useAuthStore.setState({
    auth: {
      ...savedAuth,
      user: { id: 2, username: 'admin', role: ROLE.ADMIN },
    },
  })
  const get = mockStatus()
  render(panel())
  expect(
    screen.queryByRole('button', { name: 'Backfill historical costs' })
  ).not.toBeInTheDocument()
  expect(get).not.toHaveBeenCalled()
})

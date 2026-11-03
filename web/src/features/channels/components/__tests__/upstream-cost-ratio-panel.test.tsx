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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { UpstreamCostRatioPanel } from '../upstream-cost-ratio-panel'

let client: QueryClient

const emptyHistory = {
  items: [],
  total: 0,
  page: 1,
  page_size: 10,
  sync_status: null,
}

function renderPanel(props: { syncSupported?: boolean; onApply?: () => void }) {
  const onApplyRatio = vi.fn(props.onApply)
  render(
    <QueryClientProvider client={client}>
      <UpstreamCostRatioPanel
        channelId={7}
        syncSupported={props.syncSupported ?? true}
        onApplyRatio={onApplyRatio}
      />
    </QueryClientProvider>
  )
  return { onApplyRatio }
}

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
})

test('fetching from upstream fills the resolved ratio into the form', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/7/upstream_cost_ratio/history') {
      return { data: { success: true, data: emptyHistory } }
    }
    if (url === '/api/channel/7/upstream_cost_ratio/probe') {
      return {
        data: {
          success: true,
          data: {
            group_rate_multiplier: 0.2,
            user_rate_multiplier: null,
            resolved_rate_multiplier: 0.2,
            peak_rate_enabled: false,
            observed_at: '2026-11-03T10:00:00Z',
            in_range: true,
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const user = userEvent.setup()
  const { onApplyRatio } = renderPanel({})

  await user.click(screen.getByRole('button', { name: 'Fetch from upstream' }))

  await waitFor(() => expect(onApplyRatio).toHaveBeenCalledWith(0.2))
})

test('a sync that lands out of range leaves the form ratio unchanged', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: emptyHistory },
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        status: 'out_of_range',
        old_ratio: 0.35,
        new_ratio: 0.35,
        value: 0.9,
        error: '',
      },
    },
  })
  const user = userEvent.setup()
  const { onApplyRatio } = renderPanel({})

  await user.click(screen.getByRole('button', { name: 'Sync now' }))

  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/channel/7/upstream_cost_ratio/sync')
  )
  expect(onApplyRatio).not.toHaveBeenCalled()
})

test('history entries list each change with its source and the sync status', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        items: [
          {
            id: 2,
            channel_id: 7,
            old_ratio: 0.35,
            new_ratio: 0.2,
            currency: '',
            source: 'sync',
            operator_id: 0,
            upstream_info: '{}',
            created_at: 1762160000,
          },
          {
            id: 1,
            channel_id: 7,
            old_ratio: null,
            new_ratio: 0.35,
            currency: 'USD',
            source: 'manual',
            operator_id: 1,
            upstream_info: '',
            created_at: 1762100000,
          },
        ],
        total: 2,
        page: 1,
        page_size: 10,
        sync_status: {
          status: 'failed',
          last_attempt_at: 1762160000,
          last_success_at: 0,
          last_value: null,
          error: 'upstream timeout',
        },
      },
    },
  })

  renderPanel({})

  const list = await screen.findByRole('list', { name: 'Ratio history' })
  const rows = list.querySelectorAll('li')
  expect(rows).toHaveLength(2)
  expect(rows[0]).toHaveTextContent('0.35 → 0.2')
  expect(rows[0]).toHaveTextContent('Auto sync')
  expect(rows[1]).toHaveTextContent('- → 0.35')
  expect(rows[1]).toHaveTextContent('Manual')
  expect(screen.getByText('upstream timeout')).toBeVisible()
})

test('channels without sync support show history but no sync actions', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: emptyHistory },
  })

  renderPanel({ syncSupported: false })

  expect(
    await screen.findByText('No ratio changes recorded yet')
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Sync now' })
  ).not.toBeInTheDocument()
})

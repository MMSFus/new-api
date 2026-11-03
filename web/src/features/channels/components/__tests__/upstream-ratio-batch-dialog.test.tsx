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

import type { UpstreamRatioBatchProbeItem } from '../../api'
import { UpstreamRatioBatchDialog } from '../dialogs/upstream-ratio-batch-dialog'

let client: QueryClient

function probeItem(
  overrides: Partial<UpstreamRatioBatchProbeItem>
): UpstreamRatioBatchProbeItem {
  return {
    channel_id: 1,
    name: 'supeai-a',
    type: 14,
    status: 'supported',
    resolved_rate_multiplier: 0.2,
    group_rate_multiplier: 0.2,
    user_rate_multiplier: null,
    current_ratio: 0.35,
    sync_enabled: false,
    error: '',
    ...overrides,
  }
}

function renderDialog(ids: number[]) {
  render(
    <QueryClientProvider client={client}>
      <UpstreamRatioBatchDialog
        open
        onOpenChange={() => {}}
        channelIds={ids}
        onDone={() => {}}
      />
    </QueryClientProvider>
  )
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

test('only supported channels start checked and the rest cannot be selected', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        items: [
          probeItem({ channel_id: 1, name: 'supeai-a' }),
          probeItem({
            channel_id: 2,
            name: 'official',
            status: 'ineligible',
            resolved_rate_multiplier: null,
            error: 'Official provider APIs do not expose upstream ratios',
          }),
        ],
      },
    },
  })

  renderDialog([1, 2])

  const supported = await screen.findByRole('checkbox', {
    name: '#1 supeai-a',
  })
  const ineligible = screen.getByRole('checkbox', { name: '#2 official' })
  expect(supported).toBeChecked()
  expect(ineligible).not.toBeChecked()
  expect(ineligible).toHaveAttribute('aria-disabled', 'true')
  expect(
    screen.getByRole('button', { name: 'Enable auto sync for 1 channels' })
  ).toBeEnabled()
})

test('enabling sends the checked ids with USD, the range, and sync now', async () => {
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/channel/upstream_cost_ratio/batch_probe') {
      return {
        data: {
          success: true,
          data: {
            items: [
              probeItem({ channel_id: 1 }),
              probeItem({ channel_id: 3, name: 'supeai-b' }),
            ],
          },
        },
      }
    }
    return {
      data: {
        success: true,
        data: { items: [], summary: { enabled: 1, success: 1 } },
      },
    }
  })
  const user = userEvent.setup()
  renderDialog([1, 3])

  await user.click(
    await screen.findByRole('checkbox', { name: '#3 supeai-b' })
  )
  await user.type(screen.getByLabelText('Sync range maximum'), '0.5')
  await user.click(
    screen.getByRole('button', { name: 'Enable auto sync for 1 channels' })
  )

  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/channel/upstream_cost_ratio/batch_enable',
      { ids: [1], currency: 'USD', sync_min: null, sync_max: 0.5, sync_now: true }
    )
  )
  const summary = await screen.findByLabelText('Result summary')
  expect(summary).toHaveTextContent('Auto sync enabled1')
  expect(summary).toHaveTextContent('Synced1')
})

test('a range minimum above the maximum disables enabling', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { items: [probeItem({})] } },
  })
  const user = userEvent.setup()
  renderDialog([1])

  await screen.findByRole('checkbox', { name: '#1 supeai-a' })
  await user.type(screen.getByLabelText('Sync range minimum'), '0.6')
  await user.type(screen.getByLabelText('Sync range maximum'), '0.5')

  expect(screen.getByRole('alert')).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'Enable auto sync for 1 channels' })
  ).toBeDisabled()
})

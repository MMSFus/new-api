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
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { ModelDetailsApi } from '../components/model-details-api'
import type { GroupRateLimit, PricingModel } from '../types'

const model: PricingModel = {
  id: 1,
  model_name: 'gpt-test',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: ['default', 'vip'],
}

const usableGroup = {
  default: { desc: 'Default', ratio: 1 },
  vip: { desc: 'VIP', ratio: 2 },
  svip: { desc: 'SVIP', ratio: 3 },
  auto: { desc: 'Auto', ratio: 1 },
}

let client: QueryClient | undefined

afterEach(() => {
  cleanup()
  client?.clear()
  vi.restoreAllMocks()
})

function renderRateLimits(
  groupRateLimits: Record<string, GroupRateLimit>,
  groupBalanceBuckets: Record<string, string[]>
) {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { data: {} } })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <ModelDetailsApi
        model={model}
        endpointMap={{}}
        usableGroup={usableGroup}
        groupRateLimits={groupRateLimits}
        groupBalanceBuckets={groupBalanceBuckets}
      />
    </QueryClientProvider>
  )
  const section = screen.getByText('Rate limits').closest('section')
  if (!section) throw new Error('rate limits section not rendered')
  return within(section)
}

function rowOf(view: ReturnType<typeof within>, group: string) {
  const row = view.getByText(group).closest('tr')
  if (!row) throw new Error(`row for ${group} not rendered`)
  return within(row)
}

describe('rate limits section', () => {
  it('lists only usable groups that enable the model, excluding auto', () => {
    const view = renderRateLimits({}, {})

    expect(view.getByText('default')).toBeInTheDocument()
    expect(view.getByText('vip')).toBeInTheDocument()
    expect(view.queryByText('svip')).not.toBeInTheDocument()
    expect(view.queryByText('auto')).not.toBeInTheDocument()
  })

  it('shows the window with total and success counts, and 0 as unlimited', () => {
    const view = renderRateLimits(
      { vip: { total: 0, success: 1200, duration: 5 } },
      {}
    )
    const vip = rowOf(view, 'vip')

    expect(vip.getByText('5 min')).toBeInTheDocument()
    expect(vip.getByText('Unlimited')).toBeInTheDocument()
    expect(vip.getByText('1,200')).toBeInTheDocument()
  })

  it('shows unlimited counts when no rule applies to a group', () => {
    const view = renderRateLimits({}, {})
    const row = rowOf(view, 'default')

    expect(row.getAllByText('Unlimited')).toHaveLength(2)
  })

  it('labels allowed balance types and falls back to all types when unset', () => {
    const view = renderRateLimits({}, { vip: ['topup', 'gift', 'custom'] })

    const vip = rowOf(view, 'vip')
    expect(vip.getByText('Top-up Balance')).toBeInTheDocument()
    expect(vip.getByText('Gift Balance')).toBeInTheDocument()
    expect(vip.getByText('custom')).toBeInTheDocument()
    expect(
      rowOf(view, 'default').getByText('All balance types')
    ).toBeInTheDocument()
  })
})

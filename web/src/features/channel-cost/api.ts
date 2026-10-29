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
import { t } from 'i18next'

import type { ApiResponse } from '@/features/profile/types'
import { api } from '@/lib/api'
import { createServerError } from '@/lib/server-error-message'

/** Shared by the report and log endpoints; omitted keys use backend defaults. */
export interface ChannelCostFilters {
  start_timestamp?: number
  end_timestamp?: number
  channel?: number
  model_name?: string
  username?: string
  group?: string
}

/** Quota-unit totals; refunds count negatively. */
export interface ChannelCostAmount {
  requests: number
  refunds: number
  uncosted_requests: number
  revenue: number
  costed_revenue: number
  cost: number
  profit: number
  prompt_tokens: number
  completion_tokens: number
  simulated_cache_delta: number
}

export interface ChannelCostChannel extends ChannelCostAmount {
  channel_id: number
  channel_name: string
  upstream_ratio: number | null
}

export interface ChannelCostModel extends ChannelCostAmount {
  model_name: string
}

export interface ChannelCostGroup extends ChannelCostAmount {
  group: string
}

export interface ChannelCostUser extends ChannelCostAmount {
  user_id: number
  username: string
}

export interface ChannelCostBucket extends ChannelCostAmount {
  /** Start of the hour, Unix seconds. */
  time: number
}

export interface ChannelCostReport {
  total: ChannelCostAmount
  trend: ChannelCostBucket[] | null
  channels: ChannelCostChannel[] | null
  models: ChannelCostModel[] | null
  model_count: number
  groups: ChannelCostGroup[] | null
  users: ChannelCostUser[] | null
  user_count: number
}

export interface ChannelCostSnapshot {
  upstream_ratio: number
  group_ratio: number
  base_quota: number
  cost_quota: number
}

export interface ChannelCostLog {
  id: number
  created_at: number
  /** 2 = consume, 6 = refund. */
  type: number
  user_id: number
  username: string
  model_name: string
  channel_id: number
  channel_name: string
  group: string
  quota: number
  prompt_tokens: number
  completion_tokens: number
  request_id: string
  cost: ChannelCostSnapshot | null
  simulated_cache_delta: number | null
}

export interface ChannelCostLogPage {
  items: ChannelCostLog[]
  total: number
}

/** Backend rejects ranges longer than this. */
export const CHANNEL_COST_MAX_RANGE_SECONDS = 31 * 24 * 3600

export const LOG_TYPE_REFUND = 6

export async function getChannelCostReport(
  params: ChannelCostFilters
): Promise<ChannelCostReport> {
  const response = await api.get<ApiResponse<ChannelCostReport>>(
    '/api/channel_cost/report',
    { params }
  )
  if (!response.data.success || !response.data.data) {
    throw createServerError(response.data, t('Failed to load cost report'))
  }
  return response.data.data
}

export async function getChannelCostLogs(
  params: ChannelCostFilters & { p: number; page_size: number }
): Promise<ChannelCostLogPage> {
  const response = await api.get<ApiResponse<ChannelCostLogPage>>(
    '/api/channel_cost/logs',
    { params }
  )
  if (!response.data.success || !response.data.data) {
    throw createServerError(response.data, t('Failed to load cost logs'))
  }
  return response.data.data
}

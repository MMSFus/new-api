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
import type { UpstreamCostRatioSyncStatus } from '../api'
import { CHANNEL_TYPE_ANTIGRAVITY, CHANNEL_TYPE_KIMI } from '../constants'

const CHANNEL_TYPE_AWS = 33
const CHANNEL_TYPE_VERTEX = 41
const CHANNEL_TYPE_CODEX = 57

/**
 * Official provider API domains that never answer /v1/sub2api/billing.
 * Mirrors the backend list; the backend stays the authority.
 */
export const UPSTREAM_RATIO_OFFICIAL_DOMAINS = [
  'anthropic.com',
  'openai.com',
  'googleapis.com',
  'x.ai',
  'grok.com',
  'ollama.com',
  'moonshot.cn',
  'kimi.com',
  'bigmodel.cn',
  'deepseek.com',
  'azure.com',
  'amazonaws.com',
  'cloudflare.com',
  'volces.com',
  'aliyuncs.com',
] as const

/** Channel types whose credentials are OAuth or JSON, not a plain API key. */
const CREDENTIAL_CHANNEL_TYPES = new Set<number>([
  CHANNEL_TYPE_AWS,
  CHANNEL_TYPE_VERTEX,
  CHANNEL_TYPE_CODEX,
  CHANNEL_TYPE_KIMI,
  CHANNEL_TYPE_ANTIGRAVITY,
])

export type UpstreamRatioSyncEligibility =
  | { eligible: true }
  | { eligible: false; reason: string }

export const UPSTREAM_RATIO_INELIGIBLE_REASONS = {
  NEEDS_CUSTOM_BASE_URL: 'Upstream ratio sync needs a custom base URL',
  OFFICIAL_API: 'Official provider APIs do not expose upstream ratios',
  CREDENTIALS: "This channel's credentials cannot query upstream ratios",
} as const

function isOfficialHost(hostname: string): boolean {
  const host = hostname.toLowerCase().replace(/\.$/, '')
  return UPSTREAM_RATIO_OFFICIAL_DOMAINS.some(
    (domain) => host === domain || host.endsWith(`.${domain}`)
  )
}

/**
 * Client-side preview of whether a channel can sync its upstream ratio.
 * An empty base URL is ineligible because it falls back to the official API.
 */
export function getUpstreamRatioSyncEligibility(
  type: number | undefined,
  baseUrl: string | undefined
): UpstreamRatioSyncEligibility {
  if (type !== undefined && CREDENTIAL_CHANNEL_TYPES.has(type)) {
    return {
      eligible: false,
      reason: UPSTREAM_RATIO_INELIGIBLE_REASONS.CREDENTIALS,
    }
  }
  let url: URL
  try {
    url = new URL(baseUrl?.trim() ?? '')
  } catch {
    return {
      eligible: false,
      reason: UPSTREAM_RATIO_INELIGIBLE_REASONS.NEEDS_CUSTOM_BASE_URL,
    }
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    return {
      eligible: false,
      reason: UPSTREAM_RATIO_INELIGIBLE_REASONS.NEEDS_CUSTOM_BASE_URL,
    }
  }
  if (isOfficialHost(url.hostname)) {
    return {
      eligible: false,
      reason: UPSTREAM_RATIO_INELIGIBLE_REASONS.OFFICIAL_API,
    }
  }
  return { eligible: true }
}

/** i18n keys for each sync status, shared by the panel and batch dialogs. */
export const UPSTREAM_RATIO_STATUS_LABELS: Record<
  UpstreamCostRatioSyncStatus,
  string
> = {
  success: 'Synced',
  unchanged: 'Unchanged',
  out_of_range: 'Out of range, not applied',
  unsupported: 'Upstream does not support ratio query',
  failed: 'Sync failed',
}

export const UPSTREAM_RATIO_STATUS_VARIANTS: Record<
  UpstreamCostRatioSyncStatus,
  'secondary' | 'warning' | 'destructive'
> = {
  success: 'secondary',
  unchanged: 'secondary',
  out_of_range: 'warning',
  unsupported: 'warning',
  failed: 'destructive',
}

export function formatRatio(value: number | null | undefined): string {
  return typeof value === 'number' ? String(value) : '-'
}

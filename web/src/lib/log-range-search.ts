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
import z from 'zod'

/**
 * URL search of the admin log reports (cost & billing, simulated cache).
 * A relative `range` is resolved against the time the page last searched, so
 * a reload keeps "last 7 days" instead of freezing old timestamps. Explicit
 * `start`/`end` (Unix seconds) win over `range`.
 */
export const LOG_RANGE_PRESETS = ['1h', '24h', '7d', '30d'] as const
export type LogRangePreset = (typeof LOG_RANGE_PRESETS)[number]

export const LOG_RANGE_PRESET_SECONDS: Record<LogRangePreset, number> = {
  '1h': 3600,
  '24h': 24 * 3600,
  '7d': 7 * 24 * 3600,
  '30d': 30 * 24 * 3600,
}

export const LOG_RANGE_PRESET_LABELS: Record<LogRangePreset, string> = {
  '1h': 'Last hour',
  '24h': 'Last 24 hours',
  '7d': 'Last 7 days',
  '30d': 'Last 30 days',
}

const optionalPositiveInt = z.coerce.number().int().positive().optional()
// Not trimmed: inputs are bound to the URL, trimming would eat typed spaces.
const optionalText = z.string().min(1).optional()

export const logRangeSearchSchema = z.object({
  range: z.enum(LOG_RANGE_PRESETS).optional().catch(undefined),
  start: optionalPositiveInt.catch(undefined),
  end: optionalPositiveInt.catch(undefined),
  channel: optionalPositiveInt.catch(undefined),
  user_id: optionalPositiveInt.catch(undefined),
  model: optionalText.catch(undefined),
  username: optionalText.catch(undefined),
  group: optionalText.catch(undefined),
  page: optionalPositiveInt.catch(undefined),
  pageSize: optionalPositiveInt.catch(undefined),
})

export type LogRangeSearch = z.infer<typeof logRangeSearchSchema>

/** API filter shared by both report endpoints. */
export interface LogRangeFilters {
  start_timestamp?: number
  end_timestamp?: number
  channel?: number
  user_id?: number
  model_name?: string
  username?: string
  group?: string
}

/**
 * Turns the URL search into API filters. Without explicit bounds the preset
 * (or the page default) ends at `now`; a lone bound mirrors the backend rule
 * (missing end = now, missing start = 24h before the end).
 */
export function resolveLogRangeFilters(
  search: LogRangeSearch,
  defaultPreset: LogRangePreset,
  now: number
): LogRangeFilters {
  const filters: LogRangeFilters = {}
  if (search.start === undefined && search.end === undefined) {
    const preset = search.range ?? defaultPreset
    filters.start_timestamp = now - LOG_RANGE_PRESET_SECONDS[preset]
    filters.end_timestamp = now
  } else {
    filters.end_timestamp = search.end ?? now
    filters.start_timestamp =
      search.start ?? filters.end_timestamp - LOG_RANGE_PRESET_SECONDS['24h']
  }
  if (search.channel !== undefined) filters.channel = search.channel
  if (search.user_id !== undefined) filters.user_id = search.user_id
  if (search.model !== undefined) filters.model_name = search.model
  if (search.username !== undefined) filters.username = search.username
  if (search.group !== undefined) filters.group = search.group
  return filters
}

/** Maps an API filter patch back to URL keys; empty values clear the key. */
export function logRangeFiltersToSearch(
  patch: Partial<LogRangeFilters>
): Partial<LogRangeSearch> {
  const search: Partial<LogRangeSearch> = {}
  if ('start_timestamp' in patch) search.start = patch.start_timestamp
  if ('end_timestamp' in patch) search.end = patch.end_timestamp
  if ('channel' in patch) search.channel = patch.channel
  if ('user_id' in patch) search.user_id = patch.user_id
  if ('model_name' in patch) search.model = patch.model_name || undefined
  if ('username' in patch) search.username = patch.username || undefined
  if ('group' in patch) search.group = patch.group || undefined
  return search
}

/** Active preset shown in the quick range toggle; none for custom ranges. */
export function activeLogRangePreset(
  search: LogRangeSearch,
  defaultPreset: LogRangePreset
): LogRangePreset | undefined {
  if (search.start !== undefined || search.end !== undefined) return undefined
  return search.range ?? defaultPreset
}

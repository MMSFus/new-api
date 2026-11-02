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
import type { UsageLog } from '@/features/usage-logs/data/schema'
import type { SimulatedCacheAudit } from '@/features/usage-logs/lib/simulated-cache'

import type { SimulatedCacheFilters } from './api'

export type SimulatedCacheRow = {
  log: UsageLog
  /** null if the row lacks a readable marker (should not happen per API). */
  audit: SimulatedCacheAudit | null
}

/**
 * A slice of the current search opened in the drill sheet: the whole search
 * (from a KPI card or chart), or one channel or model (from a table cell).
 */
export type SimulatedCacheScope =
  | { kind: 'all'; label: string }
  | { kind: 'channel'; channel: number; label: string }
  | { kind: 'model'; model: string; label: string }

/** Filter narrowing the page search to the scope; empty for the whole search. */
export function scopePatch(
  scope: SimulatedCacheScope
): Partial<SimulatedCacheFilters> {
  if (scope.kind === 'channel') return { channel: scope.channel }
  if (scope.kind === 'model') return { model_name: scope.model }
  return {}
}

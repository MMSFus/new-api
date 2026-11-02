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
import { useCallback, useState } from 'react'

import type { ChannelCostAmount, ChannelCostLog } from './api'
import type { ChannelCostDrillTarget } from './drill-sheet'
import type { BreakdownRow, ChannelCostDimension, TrendPoint } from './lib'
import { useDimensionLabels } from './use-dimension-labels'

/**
 * What the drill sheet shows. KPI cards open the whole search, a trend point
 * its hour or day, a breakdown row its slice, a log row its cost calculation.
 */
export function useChannelCostDrill(dimension: ChannelCostDimension) {
  const dimensionLabel = useDimensionLabels()
  const [target, setTarget] = useState<ChannelCostDrillTarget | null>(null)
  const openLog = useCallback(
    (log: ChannelCostLog) => setTarget({ kind: 'log', log }),
    []
  )
  return {
    target,
    close: () => setTarget(null),
    openLog,
    openTotal: (title: string, amount: ChannelCostAmount) =>
      setTarget({ kind: 'scope', scope: { title, patch: {}, amount } }),
    openPoint: (point: TrendPoint) =>
      setTarget({
        kind: 'scope',
        scope: {
          title: point.label,
          patch: { start_timestamp: point.start, end_timestamp: point.end },
          amount: point.amount,
        },
      }),
    openRow: (row: BreakdownRow) =>
      setTarget({
        kind: 'scope',
        scope: {
          title: `${dimensionLabel[dimension]}: ${row.label}`,
          patch: row.patch ?? {},
          amount: row.amount,
          upstreamRatio:
            dimension === 'channel' ? (row.ratio ?? null) : undefined,
        },
      }),
  }
}

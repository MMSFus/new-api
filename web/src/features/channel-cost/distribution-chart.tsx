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
import { VChart } from '@visactor/react-vchart'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatLogQuota } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import type { BreakdownRow, ChannelCostDimension } from './lib'
import { useDimensionLabels } from './use-dimension-labels'

/** Slices shown individually; the rest fold into "Other". */
const TOP_SLICES = 8
const OTHER_KEY = '__other__'

/** Revenue share donut of the breakdown rows; a slice opens its details. */
export function ChannelCostDistributionChart(props: {
  rows: BreakdownRow[]
  dimension: ChannelCostDimension
  loading: boolean
  onDrill: (row: BreakdownRow) => void
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const dimensionLabel = useDimensionLabels()
  const rows = useMemo(
    () =>
      props.rows
        .filter((row) => row.amount.revenue > 0)
        .sort((a, b) => b.amount.revenue - a.amount.revenue),
    [props.rows]
  )
  const spec = useMemo(() => {
    const values = rows.slice(0, TOP_SLICES).map((row) => ({
      key: row.key,
      type: row.label,
      value: row.amount.revenue,
    }))
    const rest = rows.slice(TOP_SLICES)
    if (rest.length > 0) {
      values.push({
        key: OTHER_KEY,
        type: t('Other'),
        value: rest.reduce((sum, row) => sum + row.amount.revenue, 0),
      })
    }
    return {
      type: 'pie' as const,
      data: [{ id: 'distribution', values }],
      outerRadius: 0.8,
      innerRadius: 0.55,
      padAngle: 0.6,
      valueField: 'value',
      categoryField: 'type',
      legends: { visible: true, orient: 'bottom' as const },
      label: { visible: false },
      tooltip: {
        mark: {
          content: [
            {
              key: (d: Record<string, unknown>) => d?.type,
              value: (d: Record<string, unknown>) =>
                formatLogQuota(Number(d?.value) || 0),
            },
          ],
        },
      },
    }
  }, [rows, t])

  let body = (
    <div className='h-64'>
      <VChart
        spec={{
          ...spec,
          theme: resolvedTheme === 'dark' ? 'dark' : 'light',
          background: 'transparent',
        }}
        option={VCHART_OPTION}
        onClick={(event: { datum?: Record<string, unknown> }) => {
          const row = rows.find((item) => item.key === event.datum?.key)
          if (row?.patch) props.onDrill(row)
        }}
      />
    </div>
  )
  if (props.loading || !themeReady) {
    body = <Skeleton className='h-64 w-full' />
  } else if (rows.length === 0) {
    body = (
      <div className='text-muted-foreground flex h-64 items-center justify-center text-sm'>
        {t('No data available')}
      </div>
    )
  }

  return (
    <figure className='flex flex-col gap-2'>
      <figcaption className='text-muted-foreground text-xs'>
        {t('Revenue share by {{dimension}}', {
          dimension: dimensionLabel[props.dimension],
        })}
      </figcaption>
      {body}
    </figure>
  )
}

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

import { PanelWrapper } from '@/features/dashboard/components/ui/panel-wrapper'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import type { RevenueBucket } from '../api'
import {
  revenueGranularity,
  revenueTrendPoints,
  type RevenueTrendPoint,
} from '../lib'
import type { RevenueFormat } from '../use-revenue-format'

/** Revenue per hour or day; the tooltip adds the compressed size. */
export function RevenueTrendChart(props: {
  trend: RevenueBucket[]
  range: { start: number; end: number }
  format: RevenueFormat
  loading: boolean
  enabled: boolean
  /** Opens the clicked hour or day. */
  onSelectPoint: (point: RevenueTrendPoint) => void
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const format = props.format
  const points = useMemo(
    () => (props.enabled ? revenueTrendPoints(props.trend, props.range) : []),
    [props.enabled, props.trend, props.range]
  )

  const revenueLabel = t('Revenue')
  const values = points.map((point) => ({
    Time: point.label,
    Metric: revenueLabel,
    Value: point.bucket.revenue,
    Bytes: point.bucket.archive_bytes,
  }))
  const spec = {
    type: 'line' as const,
    data: [{ id: 'trend', values }],
    xField: 'Time',
    yField: 'Value',
    seriesField: 'Metric',
    point: { visible: values.length <= 60 },
    line: { style: { curveType: 'monotone' as const } },
    legends: { visible: false },
    axes: [
      {
        orient: 'left' as const,
        label: {
          formatMethod: (value: unknown) => format.revenue(Number(value) || 0),
        },
      },
    ],
    tooltip: {
      mark: {
        content: [
          {
            key: revenueLabel,
            value: (d: Record<string, unknown>) =>
              format.revenue(Number(d?.Value) || 0),
          },
          {
            key: t('Compressed data'),
            value: (d: Record<string, unknown>) =>
              format.terabytes(Number(d?.Bytes) || 0),
          },
        ],
      },
    },
  }

  const selectByLabel = (label: unknown) => {
    const point = points.find((item) => item.label === label)
    if (point) props.onSelectPoint(point)
  }

  return (
    <PanelWrapper
      title={t('Revenue trend')}
      description={
        revenueGranularity(props.range) === 'day'
          ? t(
              'Days are UTC dates, as revenue is bucketed. Click a point to narrow the page to that period.'
            )
          : t('Click a point to narrow the page to that period.')
      }
      loading={props.loading || !themeReady}
      // Zero buckets are filled in; with no bucket at all there is no trend.
      empty={!props.enabled || props.trend.length === 0}
      height='h-72'
    >
      <div className='h-72'>
        <VChart
          spec={{
            ...spec,
            theme: resolvedTheme === 'dark' ? 'dark' : 'light',
            background: 'transparent',
          }}
          option={VCHART_OPTION}
          onClick={(event: { datum?: Record<string, unknown> }) =>
            selectByLabel(event.datum?.Time)
          }
          onDimensionClick={(event: {
            dimensionInfo?: { value?: unknown }[]
          }) => selectByLabel(event.dimensionInfo?.[0]?.value)}
        />
      </div>
    </PanelWrapper>
  )
}

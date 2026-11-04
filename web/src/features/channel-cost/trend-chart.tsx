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

import type { ChannelCostAmount, ChannelCostBucket } from './api'
import { formatCostQuota, trendPoints, type TrendPoint } from './lib'

export function ChannelCostTrendChart(props: {
  trend: ChannelCostBucket[]
  range: { start: number; end: number }
  loading: boolean
  /** Opens the clicked hour or day. */
  onSelectPoint: (point: TrendPoint) => void
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const points = useMemo(
    () => trendPoints(props.trend, props.range),
    [props.trend, props.range]
  )

  const spec = useMemo(() => {
    const series = [
      {
        label: t('Revenue'),
        value: (a: ChannelCostAmount) => a.costed_revenue,
      },
      { label: t('Upstream cost'), value: (a: ChannelCostAmount) => a.cost },
      { label: t('Gross profit'), value: (a: ChannelCostAmount) => a.profit },
    ]
    const values = points.flatMap((point) =>
      series.map((item) => ({
        Time: point.label,
        Bucket: point.key,
        Metric: item.label,
        Value: item.value(point.amount),
      }))
    )
    const formatValue = (datum: Record<string, unknown>) =>
      formatCostQuota(Number(datum?.Value) || 0)
    return {
      type: 'line' as const,
      data: [{ id: 'trend', values }],
      xField: 'Bucket',
      yField: 'Value',
      seriesField: 'Metric',
      point: { visible: values.length <= 60 },
      line: { style: { curveType: 'monotone' as const } },
      legends: { visible: true },
      axes: [
        {
          orient: 'bottom' as const,
          label: {
            formatMethod: (value: unknown) =>
              points.find((point) => point.key === String(value))?.label ??
              String(value),
          },
        },
        {
          orient: 'left' as const,
          label: {
            formatMethod: (value: unknown) =>
              formatCostQuota(Number(value) || 0),
          },
        },
      ],
      tooltip: {
        mark: {
          content: [
            {
              key: (d: Record<string, unknown>) => d?.Metric,
              value: formatValue,
            },
          ],
        },
        dimension: {
          content: [
            {
              key: (d: Record<string, unknown>) => d?.Metric,
              value: formatValue,
            },
          ],
        },
      },
    }
  }, [points, t])

  const selectByBucket = (bucket: unknown) => {
    const point = points.find((item) => item.key === String(bucket))
    if (point) props.onSelectPoint(point)
  }

  return (
    <PanelWrapper
      title={t('Revenue and cost trend')}
      description={t(
        'Revenue counts only requests whose cost is known, so the three lines are comparable. Click a point for its details.'
      )}
      loading={props.loading || !themeReady}
      empty={points.length === 0}
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
            selectByBucket(event.datum?.Bucket)
          }
          onDimensionClick={(event: {
            dimensionInfo?: { value?: unknown }[]
          }) => selectByBucket(event.dimensionInfo?.[0]?.value)}
        />
      </div>
    </PanelWrapper>
  )
}

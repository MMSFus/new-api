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
import dayjs from '@/lib/dayjs'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { PanelWrapper } from '@/features/dashboard/components/ui/panel-wrapper'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import type { ChannelCostAmount, ChannelCostBucket } from './api'
import { addAmounts, formatCostQuota } from './lib'

/** Ranges up to this long plot hours; longer ones fold hours into days. */
const HOURLY_RANGE_SECONDS = 2 * 24 * 3600

/** Folds hourly buckets into days in the viewer's time zone when needed. */
function trendPoints(trend: ChannelCostBucket[], rangeSeconds: number) {
  const hourly = rangeSeconds <= HOURLY_RANGE_SECONDS
  const format = hourly ? 'MM-DD HH:00' : 'YYYY-MM-DD'
  const points = new Map<string, ChannelCostAmount>()
  for (const bucket of trend) {
    const key = dayjs.unix(bucket.time).format(format)
    const previous = points.get(key)
    points.set(key, previous ? addAmounts(previous, bucket) : bucket)
  }
  return [...points.entries()]
}

export function ChannelCostTrendChart(props: {
  trend: ChannelCostBucket[]
  rangeSeconds: number
  loading: boolean
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()

  const spec = useMemo(() => {
    const series = [
      { label: t('Revenue'), value: (a: ChannelCostAmount) => a.costed_revenue },
      { label: t('Upstream cost'), value: (a: ChannelCostAmount) => a.cost },
      { label: t('Gross profit'), value: (a: ChannelCostAmount) => a.profit },
    ]
    const values = trendPoints(props.trend, props.rangeSeconds).flatMap(
      ([time, amount]) =>
        series.map((item) => ({
          Time: time,
          Metric: item.label,
          Value: item.value(amount),
        }))
    )
    const formatValue = (datum: Record<string, unknown>) =>
      formatCostQuota(Number(datum?.Value) || 0)
    return {
      type: 'line' as const,
      data: [{ id: 'trend', values }],
      xField: 'Time',
      yField: 'Value',
      seriesField: 'Metric',
      point: { visible: values.length <= 60 },
      line: { style: { curveType: 'monotone' as const } },
      legends: { visible: true },
      axes: [
        {
          orient: 'left' as const,
          label: {
            formatMethod: (value: unknown) =>
              formatCostQuota(Number(value) || 0),
          },
        },
      ],
      tooltip: {
        mark: { content: [{ key: (d: Record<string, unknown>) => d?.Metric, value: formatValue }] },
        dimension: { content: [{ key: (d: Record<string, unknown>) => d?.Metric, value: formatValue }] },
      },
    }
  }, [props.trend, props.rangeSeconds, t])

  return (
    <PanelWrapper
      title={t('Revenue and cost trend')}
      description={t(
        'Revenue counts only requests whose cost is known, so the three lines are comparable'
      )}
      loading={props.loading || !themeReady}
      empty={props.trend.length === 0}
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
        />
      </div>
    </PanelWrapper>
  )
}

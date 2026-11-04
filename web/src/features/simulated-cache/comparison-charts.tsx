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
import { PanelRightOpen } from 'lucide-react'
import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { PanelWrapper } from '@/features/dashboard/components/ui/panel-wrapper'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota, formatNumber } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import type { SimulatedCacheStat } from './api'

type ComparisonDatum = { Metric: string; Side: string; Value: number }

/** Grouped bars of upstream (original) against simulated values. */
function ComparisonBarChart(props: {
  values: ComparisonDatum[]
  format: (value: number) => string
  onClick?: () => void
}) {
  const { resolvedTheme } = useChartTheme()
  const format = props.format
  const spec = useMemo(() => {
    const formatDatum = (datum: Record<string, unknown>) =>
      format(Number(datum?.Value) || 0)
    return {
      type: 'bar' as const,
      data: [{ id: 'comparison', values: props.values }],
      xField: ['Metric', 'Side'],
      yField: 'Value',
      seriesField: 'Side',
      legends: { visible: true },
      axes: [
        {
          orient: 'left' as const,
          label: {
            formatMethod: (value: unknown) => format(Number(value) || 0),
          },
        },
      ],
      tooltip: {
        mark: {
          content: [
            {
              key: (d: Record<string, unknown>) => d?.Side,
              value: formatDatum,
            },
          ],
        },
        dimension: {
          content: [
            {
              key: (d: Record<string, unknown>) => d?.Side,
              value: formatDatum,
            },
          ],
        },
      },
    }
  }, [props.values, format])
  return (
    <div className='h-60'>
      <VChart
        spec={{
          ...spec,
          theme: resolvedTheme === 'dark' ? 'dark' : 'light',
          background: 'transparent',
        }}
        option={VCHART_OPTION}
        onClick={props.onClick}
      />
    </div>
  )
}

function DrillButton(props: { name: string; onClick: () => void }) {
  const { t } = useTranslation()
  return (
    <Button
      type='button'
      variant='ghost'
      size='icon-xs'
      aria-label={t('View details of {{name}}', { name: props.name })}
      onClick={props.onClick}
    >
      <PanelRightOpen aria-hidden='true' />
    </Button>
  )
}

export function SimulatedCacheComparisonCharts(props: {
  stat: SimulatedCacheStat | undefined
  loading: boolean
  /** A bar or the header button opens the whole search in the drill sheet. */
  onDrill: (title: string) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const { themeReady } = useChartTheme()
  const stat = props.stat
  const formatTokens = useCallback(
    (value: number) => formatNumber(value, locale),
    [locale]
  )
  const { costValues, tokenValues } = useMemo(() => {
    const original = t('Original')
    const simulated = t('Simulated')
    const cost: ComparisonDatum[] = stat
      ? [
          { Metric: t('Cost'), Side: original, Value: stat.original_quota },
          { Metric: t('Cost'), Side: simulated, Value: stat.simulated_quota },
        ]
      : []
    const tokens: ComparisonDatum[] = stat
      ? [
          {
            Metric: t('Cache Read'),
            Side: original,
            Value: stat.original_cache_read_tokens,
          },
          {
            Metric: t('Cache Read'),
            Side: simulated,
            Value: stat.simulated_cache_read_tokens,
          },
          {
            Metric: t('Cache Write'),
            Side: original,
            Value: stat.original_cache_creation_tokens,
          },
          {
            Metric: t('Cache Write'),
            Side: simulated,
            Value: stat.simulated_cache_creation_tokens,
          },
        ]
      : []
    return { costValues: cost, tokenValues: tokens }
  }, [stat, t])
  const empty = !stat || stat.count === 0
  const loading = props.loading || !themeReady

  const costTitle = t('Cost comparison')
  const tokenTitle = t('Cache token comparison')

  return (
    <div className='grid gap-4 lg:grid-cols-2'>
      <PanelWrapper
        title={costTitle}
        description={t(
          'Original is what upstream reported; Simulated is what billing charged'
        )}
        loading={loading}
        empty={empty}
        height='h-60'
        headerActions={
          <DrillButton
            name={costTitle}
            onClick={() => props.onDrill(costTitle)}
          />
        }
      >
        <ComparisonBarChart
          values={costValues}
          format={formatLogQuota}
          onClick={() => props.onDrill(costTitle)}
        />
      </PanelWrapper>
      <PanelWrapper
        title={tokenTitle}
        description={t('Tokens reported to clients as cache reads and writes')}
        loading={loading}
        empty={empty}
        height='h-60'
        headerActions={
          <DrillButton
            name={tokenTitle}
            onClick={() => props.onDrill(tokenTitle)}
          />
        }
      >
        <ComparisonBarChart
          values={tokenValues}
          format={formatTokens}
          onClick={() => props.onDrill(tokenTitle)}
        />
      </PanelWrapper>
    </div>
  )
}

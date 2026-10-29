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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ChannelCostAmount, ChannelCostReport } from './api'
import {
  formatCostQuota,
  formatPercent,
  profitMargin,
  profitTone,
} from './lib'

type Dimension = 'channel' | 'model' | 'group' | 'user'

interface BreakdownRow {
  key: string
  label: string
  /** Extra column: the channel's current upstream ratio. */
  ratio?: number | null
  amount: ChannelCostAmount
}

function breakdownRows(
  report: ChannelCostReport | undefined,
  dimension: Dimension
): BreakdownRow[] {
  if (!report) return []
  switch (dimension) {
    case 'channel':
      return (report.channels ?? []).map((row) => ({
        key: String(row.channel_id),
        label: [row.channel_name, `#${row.channel_id}`]
          .filter(Boolean)
          .join(' '),
        ratio: row.upstream_ratio,
        amount: row,
      }))
    case 'model':
      return (report.models ?? []).map((row) => ({
        key: row.model_name,
        label: row.model_name || '-',
        amount: row,
      }))
    case 'group':
      return (report.groups ?? []).map((row) => ({
        key: row.group,
        label: row.group || '-',
        amount: row,
      }))
    case 'user':
      return (report.users ?? []).map((row) => ({
        key: String(row.user_id),
        label: [row.username, `#${row.user_id}`].filter(Boolean).join(' '),
        amount: row,
      }))
  }
}

export function ChannelCostBreakdown(props: {
  report: ChannelCostReport | undefined
  loading: boolean
  /** Narrows the whole page to the clicked row. */
  onSelect: (dimension: Dimension, row: BreakdownRow) => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [dimension, setDimension] = useState<Dimension>('channel')
  const rows = breakdownRows(props.report, dimension)
  let truncatedNote: string | null = null
  if (dimension === 'model' && props.report && props.report.model_count > rows.length) {
    truncatedNote = t('Top {{shown}} of {{total}} models by revenue', {
      shown: rows.length,
      total: props.report.model_count,
    })
  } else if (dimension === 'user' && props.report && props.report.user_count > rows.length) {
    truncatedNote = t('Top {{shown}} of {{total}} users by revenue', {
      shown: rows.length,
      total: props.report.user_count,
    })
  }

  const dimensionLabel: Record<Dimension, string> = {
    channel: t('Channel'),
    model: t('Model'),
    group: t('Group'),
    user: t('User'),
  }

  return (
    <div className='bg-card overflow-hidden rounded-2xl border shadow-xs'>
      <div className='flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3 sm:px-5'>
        <div className='flex flex-col gap-1'>
          <div className='text-sm font-semibold'>{t('Breakdown')}</div>
          <div className='text-muted-foreground text-xs'>
            {truncatedNote ??
              t('Click a row to filter the page by it')}
          </div>
        </div>
        <Tabs
          value={dimension}
          onValueChange={(value) => setDimension(value as Dimension)}
        >
          <TabsList>
            {(['channel', 'model', 'group', 'user'] as const).map((value) => (
              <TabsTrigger key={value} value={value}>
                {dimensionLabel[value]}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>
      {props.loading ? (
        <div className='p-4 sm:p-5'>
          <Skeleton className='h-48 w-full' />
        </div>
      ) : (
        <StaticDataTable
          className='rounded-none border-0'
          data={rows}
          getRowKey={(row) => row.key}
          empty={rows.length === 0}
          emptyContent={t('No data available')}
          columns={[
            {
              id: 'label',
              header: dimensionLabel[dimension],
              cell: (row) => (
                <button
                  type='button'
                  className='max-w-56 truncate text-left font-medium hover:underline'
                  onClick={() => props.onSelect(dimension, row)}
                >
                  {row.label}
                </button>
              ),
            },
            ...(dimension === 'channel'
              ? [
                  {
                    id: 'ratio',
                    header: t('Upstream cost ratio'),
                    cell: (row: BreakdownRow) =>
                      row.ratio == null ? (
                        <span className='text-warning text-xs'>
                          {t('Not set')}
                        </span>
                      ) : (
                        <span className='font-mono tabular-nums'>
                          {row.ratio}
                        </span>
                      ),
                  },
                ]
              : []),
            {
              id: 'requests',
              header: t('Requests'),
              cell: (row) => (
                <span className='font-mono tabular-nums'>
                  {formatNumber(row.amount.requests, locale)}
                  {row.amount.uncosted_requests > 0 && (
                    <span
                      className='text-warning ml-1 text-xs'
                      title={t('Uncosted requests')}
                    >
                      ({formatNumber(row.amount.uncosted_requests, locale)})
                    </span>
                  )}
                </span>
              ),
            },
            {
              id: 'revenue',
              header: t('Revenue'),
              cell: (row) => (
                <span className='font-mono tabular-nums'>
                  {formatLogQuota(row.amount.revenue)}
                </span>
              ),
            },
            {
              id: 'cost',
              header: t('Upstream cost'),
              cell: (row) => (
                <span className='font-mono tabular-nums'>
                  {formatCostQuota(row.amount.cost)}
                </span>
              ),
            },
            {
              id: 'profit',
              header: t('Gross profit'),
              cell: (row) => (
                <span
                  className={cn(
                    'font-mono tabular-nums',
                    profitTone(row.amount.profit)
                  )}
                >
                  {formatCostQuota(row.amount.profit)}
                </span>
              ),
            },
            {
              id: 'margin',
              header: t('Margin'),
              cell: (row) => (
                <span className='font-mono tabular-nums'>
                  {formatPercent(profitMargin(row.amount), locale)}
                </span>
              ),
            },
          ]}
        />
      )}
    </div>
  )
}

export type { Dimension as ChannelCostDimension, BreakdownRow }

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
import { Filter } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { StaticDataTableColumn } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  formatCostQuota,
  formatPercent,
  profitMargin,
  profitTone,
  type BreakdownRow,
  type ChannelCostDimension,
} from './lib'
import { useDimensionLabels } from './use-dimension-labels'

function mono(value: string, className?: string) {
  return (
    <span className={cn('font-mono tabular-nums', className)}>{value}</span>
  )
}

/** Breakdown table columns; the name opens the drill sheet. */
export function useBreakdownColumns(options: {
  dimension: ChannelCostDimension
  onDrill: (row: BreakdownRow) => void
  onFilter: (row: BreakdownRow) => void
}): StaticDataTableColumn<BreakdownRow>[] {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const dimensionLabel = useDimensionLabels()
  const columns: StaticDataTableColumn<BreakdownRow>[] = [
    {
      id: 'label',
      header: dimensionLabel[options.dimension],
      cell: (row) =>
        row.patch ? (
          <Button
            type='button'
            variant='link'
            size='sm'
            className='h-auto max-w-56 justify-start truncate p-0 font-medium'
            onClick={() => options.onDrill(row)}
          >
            {row.label}
          </Button>
        ) : (
          <span className='max-w-56 truncate font-medium'>{row.label}</span>
        ),
    },
  ]
  if (options.dimension === 'channel') {
    columns.push({
      id: 'ratio',
      header: t('Upstream cost ratio'),
      cell: (row) =>
        row.ratio == null ? (
          <span className='text-warning text-xs'>{t('Not set')}</span>
        ) : (
          mono(String(row.ratio))
        ),
    })
  }
  columns.push(
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
      cell: (row) => mono(formatLogQuota(row.amount.revenue)),
    },
    {
      id: 'cost',
      header: t('Upstream cost'),
      cell: (row) => mono(formatCostQuota(row.amount.cost)),
    },
    {
      id: 'profit',
      header: t('Gross profit'),
      cell: (row) =>
        mono(formatCostQuota(row.amount.profit), profitTone(row.amount.profit)),
    },
    {
      id: 'margin',
      header: t('Margin'),
      cell: (row) => mono(formatPercent(profitMargin(row.amount), locale)),
    },
    {
      id: 'filter',
      header: <span className='sr-only'>{t('Filter')}</span>,
      cell: (row) =>
        row.patch && (
          <Button
            type='button'
            variant='ghost'
            size='icon-xs'
            aria-label={t('Filter by {{name}}', { name: row.label })}
            onClick={() => options.onFilter(row)}
          >
            <Filter aria-hidden='true' />
          </Button>
        ),
    }
  )
  return columns
}

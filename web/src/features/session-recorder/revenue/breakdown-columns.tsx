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
import { DrillCell } from '@/features/simulated-cache/drill-cell'

import type { RevenueByModel } from '../api'
import type { RevenueFormat } from '../use-revenue-format'

/** A by-rule row; `rule_id` is null for the records no rule matched. */
export interface RuleRow {
  rule_id: string | null
  rule_name: string
  price_per_tb: number | null
  records: number
  archive_bytes: number
  revenue: number
}

function mono(value: string) {
  return <span className='font-mono tabular-nums'>{value}</span>
}

export function useRuleColumns(options: {
  format: RevenueFormat
  onDrill: (row: RuleRow) => void
}): StaticDataTableColumn<RuleRow>[] {
  const { t } = useTranslation()
  const format = options.format
  return [
    {
      id: 'rule',
      header: t('Rule'),
      cell: (row) => (
        <DrillCell
          label={row.rule_name}
          title={row.rule_name}
          onClick={() => options.onDrill(row)}
        />
      ),
    },
    {
      id: 'price',
      header: t('Price per TB'),
      cell: (row) =>
        row.price_per_tb === null
          ? '-'
          : mono(format.revenue(row.price_per_tb)),
    },
    {
      id: 'records',
      header: t('Records'),
      cell: (row) => mono(format.count(row.records)),
    },
    {
      id: 'archive',
      header: t('Compressed size'),
      cell: (row) => mono(format.terabytes(row.archive_bytes)),
    },
    {
      id: 'revenue',
      header: t('Revenue'),
      cell: (row) => mono(format.revenue(row.revenue)),
    },
  ]
}

export function useModelColumns(options: {
  format: RevenueFormat
  ruleNames: Map<string, string>
  onDrill: (row: RevenueByModel) => void
  onFilter: (row: RevenueByModel) => void
}): StaticDataTableColumn<RevenueByModel>[] {
  const { t } = useTranslation()
  const format = options.format
  return [
    {
      id: 'model',
      header: t('Model'),
      cell: (row) => (
        <DrillCell
          label={row.model_name || '-'}
          title={row.model_name}
          onClick={() => options.onDrill(row)}
        />
      ),
    },
    {
      id: 'rule',
      header: t('Matched rule'),
      cell: (row) =>
        row.rule_id ? (
          (options.ruleNames.get(row.rule_id) ?? row.rule_id)
        ) : (
          <span className='text-muted-foreground'>{t('Unmatched')}</span>
        ),
    },
    {
      id: 'records',
      header: t('Records'),
      cell: (row) => mono(format.count(row.records)),
    },
    {
      id: 'raw',
      header: t('Raw size'),
      cell: (row) => mono(format.bytes(row.raw_bytes)),
    },
    {
      id: 'archive',
      header: t('Compressed size'),
      cell: (row) => mono(format.terabytes(row.archive_bytes)),
    },
    {
      id: 'revenue',
      header: t('Revenue'),
      cell: (row) => mono(format.revenue(row.revenue)),
    },
    {
      id: 'filter',
      header: <span className='sr-only'>{t('Filter')}</span>,
      cell: (row) =>
        row.model_name && (
          <Button
            type='button'
            variant='ghost'
            size='icon-xs'
            aria-label={t('Filter by {{name}}', { name: row.model_name })}
            onClick={() => options.onFilter(row)}
          >
            <Filter aria-hidden='true' />
          </Button>
        ),
    },
  ]
}

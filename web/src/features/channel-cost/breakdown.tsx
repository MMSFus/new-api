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
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Skeleton } from '@/components/ui/skeleton'

import type { ChannelCostReport } from './api'
import { useBreakdownColumns } from './breakdown-columns'
import { ChannelCostDimensionTabs } from './dimension-tabs'
import { ChannelCostDistributionChart } from './distribution-chart'
import {
  breakdownRows,
  type BreakdownRow,
  type ChannelCostDimension,
} from './lib'

/**
 * Revenue share chart and breakdown table of one dimension. Both follow the
 * same dimension tabs; a slice or a name opens the drill sheet.
 */
export function ChannelCostBreakdown(props: {
  report: ChannelCostReport | undefined
  loading: boolean
  dimension: ChannelCostDimension
  onDimensionChange: (dimension: ChannelCostDimension) => void
  /** Opens the row in the drill sheet. */
  onDrill: (row: BreakdownRow) => void
  /** Narrows the whole page to the row. */
  onFilter: (row: BreakdownRow) => void
}) {
  const { t } = useTranslation()
  const dimension = props.dimension
  const report = props.report
  const rows = breakdownRows(report, dimension)
  const columns = useBreakdownColumns({
    dimension,
    onDrill: props.onDrill,
    onFilter: props.onFilter,
  })
  let note = t(
    'Click a slice or a name for its details, or the filter icon to filter the page'
  )
  if (dimension === 'model' && report && report.model_count > rows.length) {
    note = t('Top {{shown}} of {{total}} models by revenue', {
      shown: rows.length,
      total: report.model_count,
    })
  } else if (
    dimension === 'user' &&
    report &&
    report.user_count > rows.length
  ) {
    note = t('Top {{shown}} of {{total}} users by revenue', {
      shown: rows.length,
      total: report.user_count,
    })
  }

  return (
    <section
      aria-label={t('Breakdown')}
      className='bg-card overflow-hidden rounded-2xl border shadow-xs'
    >
      <div className='flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3 sm:px-5'>
        <div className='flex min-w-0 flex-col gap-1'>
          <h2 className='text-sm font-semibold'>{t('Breakdown')}</h2>
          <p className='text-muted-foreground text-xs'>{note}</p>
        </div>
        <ChannelCostDimensionTabs
          value={dimension}
          onValueChange={props.onDimensionChange}
        />
      </div>
      <div className='grid xl:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]'>
        <div className='border-b p-4 sm:p-5 xl:border-r xl:border-b-0'>
          <ChannelCostDistributionChart
            rows={rows}
            dimension={dimension}
            loading={props.loading}
            onDrill={props.onDrill}
          />
        </div>
        <div className='min-w-0'>
          {props.loading ? (
            <div className='p-4 sm:p-5'>
              <Skeleton className='h-64 w-full' />
            </div>
          ) : (
            <StaticDataTable
              className='rounded-none border-0'
              data={rows}
              getRowKey={(row) => row.key}
              empty={rows.length === 0}
              emptyContent={t('No data available')}
              columns={columns}
            />
          )}
        </div>
      </div>
    </section>
  )
}

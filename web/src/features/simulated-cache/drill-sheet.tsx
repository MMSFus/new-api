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
import type { PaginationState } from '@tanstack/react-table'
import { Filter } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import dayjs from '@/lib/dayjs'

import type { SimulatedCacheFilters } from './api'
import { useSimulatedCacheColumns } from './columns'
import { SimulatedCacheLogsTable } from './logs-table'
import { SimulatedCacheSummaryCards } from './summary-cards'
import { scopePatch, type SimulatedCacheScope } from './types'
import { useSimulatedCacheData } from './use-simulated-cache-data'
import { useSimulatedCacheTable } from './use-simulated-cache-table'

/**
 * Drill-down of the whole search or one channel or model inside it: totals,
 * then each request, expandable into its original/simulated calculation.
 */
export function SimulatedCacheDrillSheet(props: {
  scope: SimulatedCacheScope | null
  filters: SimulatedCacheFilters
  onOpenChange: (open: boolean) => void
  onApplyFilter: (patch: Partial<SimulatedCacheFilters>) => void
}) {
  return (
    <Sheet open={props.scope !== null} onOpenChange={props.onOpenChange}>
      <SheetContent
        className={sideDrawerContentClassName('sm:max-w-3xl xl:max-w-5xl')}
      >
        {props.scope && (
          <DrillContent
            key={`${props.scope.kind}:${props.scope.label}`}
            scope={props.scope}
            filters={props.filters}
            onApplyFilter={props.onApplyFilter}
          />
        )}
      </SheetContent>
    </Sheet>
  )
}

function DrillContent(props: {
  scope: SimulatedCacheScope
  filters: SimulatedCacheFilters
  onApplyFilter: (patch: Partial<SimulatedCacheFilters>) => void
}) {
  const { t } = useTranslation()
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 10,
  })
  const patch = scopePatch(props.scope)
  const filters = { ...props.filters, ...patch }
  const data = useSimulatedCacheData(filters, pagination, true)
  const columns = useSimulatedCacheColumns()
  const table = useSimulatedCacheTable({
    columns,
    rows: data.rows,
    total: data.total,
    pagination,
    onPaginationChange: setPagination,
    locked: data.listQuery.isFetching,
  })
  let title = props.scope.label
  if (props.scope.kind === 'channel') title = `${t('Channel')}: ${title}`
  else if (props.scope.kind === 'model') title = `${t('Model')}: ${title}`

  return (
    <>
      <SheetHeader className={sideDrawerHeaderClassName('pr-12')}>
        <SheetTitle>{title}</SheetTitle>
        <SheetDescription className='tabular-nums'>
          {dayjs.unix(filters.start_timestamp ?? 0).format('YYYY-MM-DD HH:mm')}
          {' ~ '}
          {dayjs.unix(filters.end_timestamp ?? 0).format('YYYY-MM-DD HH:mm')}
        </SheetDescription>
      </SheetHeader>
      <div className={sideDrawerFormClassName('gap-4')}>
        {props.scope.kind !== 'all' && (
          <div className='flex justify-end'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => props.onApplyFilter(patch)}
            >
              <Filter aria-hidden='true' />
              {t('Apply as filter')}
            </Button>
          </div>
        )}
        {data.statQuery.isError || data.listQuery.isError ? (
          <ErrorState
            title={t('Failed to load simulated cache logs')}
            description={data.error?.message}
            onRetry={data.refetch}
          />
        ) : (
          <>
            <SimulatedCacheSummaryCards
              stat={data.statQuery.data}
              loading={data.statQuery.isPending}
              error={false}
            />
            <SimulatedCacheLogsTable
              table={table}
              columns={columns}
              isLoading={data.listQuery.isPending}
              isFetching={data.listQuery.isFetching}
              isError={false}
            />
          </>
        )}
      </div>
    </>
  )
}

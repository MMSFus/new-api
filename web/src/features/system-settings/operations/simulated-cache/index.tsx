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
import { useQuery } from '@tanstack/react-query'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePage,
  DataTableRow,
  useDataTable,
} from '@/components/data-table'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { TableCell, TableRow } from '@/components/ui/table'
import { SimulatedCacheComparison } from '@/features/usage-logs/components/simulated-cache-comparison'
import { parseLogOther } from '@/features/usage-logs/lib/format'
import { getSimulatedCache } from '@/features/usage-logs/lib/simulated-cache'
import { requireServerSuccess } from '@/lib/server-error-message'

import { SettingsSection } from '../../components/settings-section'
import {
  getSimulatedCacheLogs,
  getSimulatedCacheStat,
  SIMULATED_CACHE_MAX_RANGE_SECONDS,
  type SimulatedCacheFilters,
} from './api'
import { useSimulatedCacheColumns, type SimulatedCacheRow } from './columns'
import { SimulatedCacheFilterBar } from './filter-bar'
import { SimulatedCacheSummaryCards } from './summary-cards'

const DEFAULT_RANGE_SECONDS = 24 * 3600
const EMPTY_ROWS: SimulatedCacheRow[] = []

function lastDayFilters(): SimulatedCacheFilters {
  const now = Math.floor(Date.now() / 1000)
  return {
    start_timestamp: now - DEFAULT_RANGE_SECONDS,
    end_timestamp: now,
  }
}

/**
 * Admin audit of requests whose upstream prompt cache usage was replaced by a
 * simulated one (channel setting `simulated_cache_enabled`).
 */
export function SimulatedCacheSection() {
  const { t } = useTranslation()
  const [filters, setFilters] = useState<SimulatedCacheFilters>(lastDayFilters)
  const [page, setPage] = useState({ p: 1, page_size: 20 })

  // Mirror the backend rule: missing bounds default to the last 24h ending now.
  const effectiveEnd = filters.end_timestamp ?? Math.floor(Date.now() / 1000)
  const effectiveStart =
    filters.start_timestamp ?? effectiveEnd - DEFAULT_RANGE_SECONDS
  let rangeError = ''
  if (effectiveStart > effectiveEnd) {
    rangeError = t('End time must be after start time')
  } else if (
    effectiveEnd - effectiveStart >
    SIMULATED_CACHE_MAX_RANGE_SECONDS
  ) {
    rangeError = t('Time range cannot exceed 7 days')
  }
  const canQuery = rangeError === ''

  const statQuery = useQuery({
    queryKey: ['simulated-cache', 'stat', filters],
    queryFn: async () =>
      requireServerSuccess(await getSimulatedCacheStat(filters)),
    enabled: canQuery,
    retry: false,
  })
  const listParams = { ...filters, ...page }
  const listQuery = useQuery({
    queryKey: ['simulated-cache', 'logs', listParams],
    queryFn: async () =>
      requireServerSuccess(await getSimulatedCacheLogs(listParams)),
    enabled: canQuery,
    retry: false,
  })
  const rows =
    canQuery && listQuery.data
      ? listQuery.data.items.map((log) => ({
          log,
          audit: getSimulatedCache(parseLogOther(log.other)),
        }))
      : EMPTY_ROWS

  const columns = useSimulatedCacheColumns()
  const { table } = useDataTable({
    columns,
    data: rows,
    getRowId: (row) => String(row.log.id),
    totalCount:
      canQuery && !listQuery.isError ? (listQuery.data?.total ?? 0) : 0,
    pagination: { pageIndex: page.p - 1, pageSize: page.page_size },
    onPaginationChange: (updater) => {
      if (listQuery.isFetching || !canQuery) return
      setPage((previous) => {
        const current = {
          pageIndex: previous.p - 1,
          pageSize: previous.page_size,
        }
        const next = typeof updater === 'function' ? updater(current) : updater
        return {
          p: next.pageSize === previous.page_size ? next.pageIndex + 1 : 1,
          page_size: next.pageSize,
        }
      })
    },
    enableRowSelection: false,
    enableSorting: false,
    manualFiltering: true,
    manualPagination: true,
  })
  const updateFilters = (patch: Partial<SimulatedCacheFilters>) => {
    setFilters((previous) => ({ ...previous, ...patch }))
    setPage((previous) => ({ ...previous, p: 1 }))
  }
  const refetchAll = () => {
    if (!canQuery) return
    void statQuery.refetch()
    void listQuery.refetch()
  }

  return (
    <SettingsSection title={t('Simulated cache')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Requests on channels with a simulated prompt cache. Original is what upstream reported; Simulated is what the client saw and billing charged.'
        )}
      </p>
      <SimulatedCacheSummaryCards
        stat={canQuery ? statQuery.data : undefined}
        loading={canQuery && statQuery.isPending}
        error={!canQuery || statQuery.isError}
      />
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={canQuery && listQuery.isPending}
        isFetching={listQuery.isFetching}
        emptyTitle={
          listQuery.isError
            ? t('Failed to load simulated cache logs')
            : t('No records')
        }
        hideMobile
        fixedHeight={false}
        paginationInFooter={false}
        applyHeaderSize
        toolbar={
          <div className='space-y-2'>
            <SimulatedCacheFilterBar
              table={table}
              filters={filters}
              onChange={updateFilters}
              isFetching={listQuery.isFetching || statQuery.isFetching}
              onSearch={refetchAll}
              onReset={() => {
                setFilters(lastDayFilters())
                setPage((previous) => ({ ...previous, p: 1 }))
              }}
            />
            {rangeError && (
              <Alert variant='destructive'>
                <AlertDescription>{rangeError}</AlertDescription>
              </Alert>
            )}
            {canQuery && (listQuery.isError || statQuery.isError) && (
              <Alert variant='destructive'>
                <AlertDescription className='flex items-center justify-between gap-2'>
                  <span>
                    {(listQuery.error ?? statQuery.error)?.message ||
                      t('Failed to load simulated cache logs')}
                  </span>
                  <Button size='sm' variant='outline' onClick={refetchAll}>
                    {t('Retry')}
                  </Button>
                </AlertDescription>
              </Alert>
            )}
          </div>
        }
        renderRow={(row) => (
          <Fragment key={row.id}>
            {/* className varies with expansion so the memoized row re-renders
                its toggle; the row object itself stays stable. */}
            <DataTableRow
              row={row}
              className={row.getIsExpanded() ? 'bg-muted/30' : undefined}
              cellRenderColumns={table.options.columns}
            />
            {row.getIsExpanded() && row.original.audit && (
              <TableRow className='hover:bg-transparent'>
                <TableCell
                  colSpan={row.getVisibleCells().length}
                  className='bg-muted/30 whitespace-normal'
                >
                  <SimulatedCacheComparison audit={row.original.audit} />
                </TableCell>
              </TableRow>
            )}
          </Fragment>
        )}
      />
    </SettingsSection>
  )
}

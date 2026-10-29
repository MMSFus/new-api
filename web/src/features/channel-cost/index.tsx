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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { SimulatedCacheFilterBar } from '@/features/simulated-cache/filter-bar'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  CHANNEL_COST_MAX_RANGE_SECONDS,
  getChannelCostLogs,
  getChannelCostReport,
  type ChannelCostFilters,
  type ChannelCostLog,
} from './api'
import { ChannelCostBreakdown } from './breakdown'
import { useChannelCostColumns } from './columns'
import { ChannelCostSummaryCards } from './summary-cards'
import { ChannelCostTrendChart } from './trend-chart'

const DEFAULT_RANGE_SECONDS = 7 * 24 * 3600
const EMPTY_LOGS: ChannelCostLog[] = []

function lastWeekFilters(): ChannelCostFilters {
  const now = Math.floor(Date.now() / 1000)
  return {
    start_timestamp: now - DEFAULT_RANGE_SECONDS,
    end_timestamp: now,
  }
}

/**
 * Revenue, upstream cost and gross profit from consume and refund logs. The
 * cost of each log is the snapshot taken with the channel's
 * `upstream_cost_ratio` when the log was written.
 */
export function ChannelCost() {
  const { t } = useTranslation()
  const [filters, setFilters] = useState<ChannelCostFilters>(lastWeekFilters)
  const [page, setPage] = useState({ p: 1, page_size: 20 })

  // Mirror the backend rule: a missing end is now, a missing start 24h before.
  const effectiveEnd = filters.end_timestamp ?? Math.floor(Date.now() / 1000)
  const effectiveStart = filters.start_timestamp ?? effectiveEnd - 24 * 3600
  let rangeError = ''
  if (effectiveStart > effectiveEnd) {
    rangeError = t('End time must be after start time')
  } else if (effectiveEnd - effectiveStart > CHANNEL_COST_MAX_RANGE_SECONDS) {
    rangeError = t('Time range cannot exceed {{days}} days', {
      days: CHANNEL_COST_MAX_RANGE_SECONDS / (24 * 3600),
    })
  }
  const canQuery = rangeError === ''

  const reportQuery = useQuery({
    queryKey: ['channel-cost', 'report', filters],
    queryFn: async () =>
      requireServerSuccess(await getChannelCostReport(filters)),
    enabled: canQuery,
    retry: false,
  })
  const listParams = { ...filters, ...page }
  const listQuery = useQuery({
    queryKey: ['channel-cost', 'logs', listParams],
    queryFn: async () =>
      requireServerSuccess(await getChannelCostLogs(listParams)),
    enabled: canQuery,
    retry: false,
  })
  const report = canQuery ? reportQuery.data : undefined

  const columns = useChannelCostColumns()
  const { table } = useDataTable({
    columns,
    data: canQuery && listQuery.data ? listQuery.data.items : EMPTY_LOGS,
    getRowId: (log) => String(log.id),
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
  const updateFilters = (patch: Partial<ChannelCostFilters>) => {
    setFilters((previous) => ({ ...previous, ...patch }))
    setPage((previous) => ({ ...previous, p: 1 }))
  }
  const refetchAll = () => {
    if (!canQuery) return
    void reportQuery.refetch()
    void listQuery.refetch()
  }
  const queryError = canQuery && (reportQuery.isError || listQuery.isError)

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Cost & Billing')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              "Cost is the official price times the channel's upstream cost ratio at the time of the request. Requests on channels without a ratio are uncosted and left out of profit and margin."
            )}
          </p>
          <SimulatedCacheFilterBar
            table={table}
            filters={filters}
            onChange={updateFilters}
            withGroup
            isFetching={reportQuery.isFetching || listQuery.isFetching}
            onSearch={refetchAll}
            onReset={() => {
              setFilters(lastWeekFilters())
              setPage((previous) => ({ ...previous, p: 1 }))
            }}
          />
          {rangeError && (
            <Alert variant='destructive'>
              <AlertDescription>{rangeError}</AlertDescription>
            </Alert>
          )}
          {queryError && (
            <Alert variant='destructive'>
              <AlertDescription className='flex items-center justify-between gap-2'>
                <span>
                  {(reportQuery.error ?? listQuery.error)?.message ||
                    t('Failed to load cost report')}
                </span>
                <Button size='sm' variant='outline' onClick={refetchAll}>
                  {t('Retry')}
                </Button>
              </AlertDescription>
            </Alert>
          )}
          <ChannelCostSummaryCards
            total={report?.total}
            loading={canQuery && reportQuery.isPending}
            error={!canQuery || reportQuery.isError}
          />
          <ChannelCostTrendChart
            trend={report?.trend ?? []}
            rangeSeconds={effectiveEnd - effectiveStart}
            loading={canQuery && reportQuery.isPending}
          />
          <ChannelCostBreakdown
            report={report}
            loading={canQuery && reportQuery.isPending}
            onSelect={(dimension, row) => {
              if (dimension === 'channel') {
                updateFilters({ channel: Number(row.key) || undefined })
              } else if (dimension === 'model') {
                updateFilters({ model_name: row.key || undefined })
              } else if (dimension === 'group') {
                updateFilters({ group: row.key || undefined })
              } else {
                const user = report?.users?.find(
                  (item) => String(item.user_id) === row.key
                )
                updateFilters({ username: user?.username || undefined })
              }
            }}
          />
          <DataTablePage
            table={table}
            columns={columns}
            isLoading={canQuery && listQuery.isPending}
            isFetching={listQuery.isFetching}
            emptyTitle={
              listQuery.isError ? t('Failed to load cost logs') : t('No records')
            }
            hideMobile
            fixedHeight={false}
            paginationInFooter={false}
            applyHeaderSize
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

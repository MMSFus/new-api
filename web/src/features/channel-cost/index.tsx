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
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { SimulatedCacheFilterBar } from '@/features/simulated-cache/filter-bar'
import { useLogRangeSearch } from '@/hooks/use-log-range-search'

import { ChannelCostBackfillEntry } from './backfill-entry'
import { ChannelCostBreakdown } from './breakdown'
import { useChannelCostColumns } from './columns'
import { ChannelCostDrillSheet } from './drill-sheet'
import { ChannelCostLogsTable } from './logs-table'
import { ChannelCostSummaryCards } from './summary-cards'
import { ChannelCostTrendChart } from './trend-chart'
import {
  useChannelCostLogs,
  useChannelCostRangeError,
  useChannelCostReport,
} from './use-channel-cost-data'
import { useChannelCostDrill } from './use-channel-cost-drill'
import { useChannelCostTable } from './use-channel-cost-table'

const route = getRouteApi('/_authenticated/channel-cost/')
const QUICK_RANGES = ['24h', '7d', '30d'] as const

/**
 * Revenue, upstream cost and gross profit from consume and refund logs. The
 * cost of each log is the snapshot taken with the channel's
 * `upstream_cost_ratio` when the log was written. Filters live in the URL;
 * KPIs, charts and the breakdown open their slice in the drill sheet.
 */
export function ChannelCost() {
  const { t } = useTranslation()
  const routeSearch = route.useSearch()
  const navigate = route.useNavigate()
  const search = useLogRangeSearch({
    search: routeSearch,
    navigate,
    defaultPreset: '7d',
  })
  const dimension = routeSearch.dimension ?? 'channel'
  const drill = useChannelCostDrill(dimension)
  const filters = search.filters
  const rangeError = useChannelCostRangeError(filters)
  const canQuery = rangeError === ''
  const reportQuery = useChannelCostReport(filters, canQuery)
  const logs = useChannelCostLogs(filters, search.pagination, canQuery)
  const report = canQuery ? reportQuery.data : undefined
  const columns = useChannelCostColumns(drill.openLog)
  const table = useChannelCostTable({
    columns,
    logs: logs.logs,
    total: logs.total,
    pagination: search.pagination,
    onPaginationChange: search.onPaginationChange,
    locked: logs.query.isFetching || !canQuery,
  })
  const refetchAll = () => {
    if (!canQuery) return
    void reportQuery.refetch()
    void logs.query.refetch()
  }
  const onSearch = () => {
    if (!search.refreshNow()) refetchAll()
  }
  const queryError = canQuery ? (reportQuery.error ?? logs.query.error) : null
  const reportLoading = canQuery && reportQuery.isPending
  const range = {
    start: filters.start_timestamp ?? 0,
    end: filters.end_timestamp ?? 0,
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Cost & Billing')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <ChannelCostBackfillEntry
          filters={filters}
          missing={report?.total.uncosted_requests}
          enabled={canQuery && !reportQuery.isFetching && !reportQuery.isError}
        />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Costs use channel ratios saved at request time. Historical backfill estimates costs using ratios at task start and labels the results. Uncosted requests are excluded from profit and margin.'
            )}
          </p>
          <SimulatedCacheFilterBar
            table={table}
            filters={filters}
            onChange={search.updateFilters}
            withGroup
            isFetching={reportQuery.isFetching || logs.query.isFetching}
            onSearch={onSearch}
            onReset={search.reset}
            presets={QUICK_RANGES}
            preset={search.preset}
            onPresetChange={search.selectPreset}
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
                  {queryError.message || t('Failed to load cost report')}
                </span>
                <Button size='sm' variant='outline' onClick={refetchAll}>
                  {t('Retry')}
                </Button>
              </AlertDescription>
            </Alert>
          )}
          <ChannelCostSummaryCards
            total={report?.total}
            loading={reportLoading}
            error={!canQuery || reportQuery.isError}
            onDrill={
              report
                ? (title) => drill.openTotal(title, report.total)
                : undefined
            }
          />
          <ChannelCostTrendChart
            trend={report?.trend ?? []}
            range={range}
            loading={reportLoading}
            onSelectPoint={drill.openPoint}
          />
          <ChannelCostBreakdown
            report={report}
            loading={reportLoading}
            dimension={dimension}
            onDimensionChange={(next) =>
              navigate({
                replace: true,
                search: (prev) => ({
                  ...prev,
                  dimension: next === 'channel' ? undefined : next,
                }),
              })
            }
            onDrill={drill.openRow}
            onFilter={(row) => row.patch && search.updateFilters(row.patch)}
          />
          <ChannelCostLogsTable
            table={table}
            columns={columns}
            isLoading={canQuery && logs.query.isPending}
            isFetching={logs.query.isFetching}
            isError={logs.query.isError}
          />
          <ChannelCostDrillSheet
            target={drill.target}
            filters={filters}
            onOpenChange={(open) => {
              if (!open) drill.close()
            }}
            onApplyFilter={(patch) => {
              drill.close()
              search.updateFilters(patch)
            }}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

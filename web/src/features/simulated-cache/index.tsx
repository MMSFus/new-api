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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useLogRangeSearch } from '@/hooks/use-log-range-search'

import { useSimulatedCacheColumns } from './columns'
import { SimulatedCacheComparisonCharts } from './comparison-charts'
import { SimulatedCacheDrillSheet } from './drill-sheet'
import { SimulatedCacheFilterBar } from './filter-bar'
import { SimulatedCacheLogsTable } from './logs-table'
import { SimulatedCacheSummaryCards } from './summary-cards'
import type { SimulatedCacheScope } from './types'
import {
  useSimulatedCacheData,
  useSimulatedCacheRangeError,
} from './use-simulated-cache-data'
import { useSimulatedCacheTable } from './use-simulated-cache-table'

const route = getRouteApi('/_authenticated/simulated-cache/')
const QUICK_RANGES = ['1h', '24h', '7d'] as const

/**
 * Admin audit of requests whose upstream prompt cache usage was replaced by a
 * simulated one (channel setting `simulated_cache_enabled`). Filters live in
 * the URL; KPIs and charts sit above the audit table, and each of them opens
 * the matching requests in the drill sheet.
 */
export function SimulatedCache() {
  const { t } = useTranslation()
  const search = useLogRangeSearch({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    defaultPreset: '24h',
  })
  const [scope, setScope] = useState<SimulatedCacheScope | null>(null)
  const rangeError = useSimulatedCacheRangeError(search.filters)
  const canQuery = rangeError === ''
  const data = useSimulatedCacheData(
    search.filters,
    search.pagination,
    canQuery
  )
  const columns = useSimulatedCacheColumns(setScope)
  const table = useSimulatedCacheTable({
    columns,
    rows: data.rows,
    total: data.total,
    pagination: search.pagination,
    onPaginationChange: search.onPaginationChange,
    locked: data.listQuery.isFetching || !canQuery,
  })
  const openAll = (label: string) => setScope({ kind: 'all', label })
  const stat = canQuery ? data.statQuery.data : undefined
  const statLoading = canQuery && data.statQuery.isPending
  const onSearch = () => {
    if (search.commitFilters()) return
    if (!search.refreshNow()) data.refetch()
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Simulated cache')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Requests on channels with a simulated prompt cache. Original is what upstream reported; Simulated is what the client saw and billing charged.'
            )}
          </p>
          <SimulatedCacheFilterBar
            table={table}
            filters={search.draftFilters}
            onChange={search.editFilters}
            isFetching={data.isFetching}
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
          {data.error && (
            <Alert variant='destructive'>
              <AlertDescription className='flex items-center justify-between gap-2'>
                <span>
                  {data.error.message ||
                    t('Failed to load simulated cache logs')}
                </span>
                <Button size='sm' variant='outline' onClick={data.refetch}>
                  {t('Retry')}
                </Button>
              </AlertDescription>
            </Alert>
          )}
          <SimulatedCacheSummaryCards
            stat={stat}
            loading={statLoading}
            error={!canQuery || data.statQuery.isError}
            onDrill={stat ? openAll : undefined}
          />
          <SimulatedCacheComparisonCharts
            stat={stat}
            loading={statLoading}
            onDrill={openAll}
          />
          <SimulatedCacheLogsTable
            table={table}
            columns={columns}
            isLoading={canQuery && data.listQuery.isPending}
            isFetching={data.listQuery.isFetching}
            isError={data.listQuery.isError}
          />
          <SimulatedCacheDrillSheet
            scope={scope}
            filters={search.filters}
            onOpenChange={(open) => {
              if (!open) setScope(null)
            }}
            onApplyFilter={(patch) => {
              setScope(null)
              search.updateFilters(patch)
            }}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

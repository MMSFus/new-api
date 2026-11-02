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
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useLogRangeError } from '@/hooks/use-log-range-search'
import { useAuthStore } from '@/stores/auth-store'

import { BackfillEntry } from './backfill/backfill-entry'
import { ContentTab } from './content/content-tab'
import { SessionRecorderFilterBar } from './filter-bar'
import {
  allowedTabs,
  sessionRecorderAccess,
  type SessionRecorderTab,
} from './lib'
import { REVENUE_MAX_RANGE_SECONDS, RevenueTab } from './revenue/revenue-tab'
import { SESSION_RECORDER_QUERY_KEY } from './use-session-recorder-data'
import { useSessionRecorderSearch } from './use-session-recorder-search'

const TAB_LABELS: Record<SessionRecorderTab, string> = {
  revenue: 'Revenue details',
  content: 'Conversation review',
}

/**
 * Session recorder console: revenue of archived recordings priced by model
 * rules, and review of recorded conversations. Each tab needs its own
 * permission; the filters live in the URL.
 */
export function SessionRecorder() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const access = sessionRecorderAccess(user)
  const tabs = allowedTabs(access)
  const search = useSessionRecorderSearch()
  const requested = search.routeSearch.tab
  const tab = requested && tabs.includes(requested) ? requested : tabs[0]
  const filters = search.range.filters
  const revenueRangeError = useLogRangeError(filters, REVENUE_MAX_RANGE_SECONDS)
  const contentRangeError = useLogRangeError(filters, Number.POSITIVE_INFINITY)
  const rangeError = tab === 'revenue' ? revenueRangeError : contentRangeError
  const fetching = useIsFetching({ queryKey: SESSION_RECORDER_QUERY_KEY }) > 0
  const range = {
    start: filters.start_timestamp ?? 0,
    end: filters.end_timestamp ?? 0,
  }

  const onSearch = () => {
    if (search.range.refreshNow()) return
    void queryClient.refetchQueries({
      queryKey: SESSION_RECORDER_QUERY_KEY,
      type: 'active',
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Session Recorder')}</SectionPageLayout.Title>
      {access.backfill && (
        <SectionPageLayout.Actions>
          <BackfillEntry />
        </SectionPageLayout.Actions>
      )}
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <Tabs
            value={tab}
            onValueChange={(value) =>
              search.selectTab(value as SessionRecorderTab)
            }
          >
            <TabsList aria-label={t('Session recorder sections')}>
              {tabs.map((item) => (
                <TabsTrigger key={item} value={item}>
                  {t(TAB_LABELS[item])}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          {tab && (
            <SessionRecorderFilterBar
              search={search}
              tab={tab}
              isFetching={fetching}
              onSearch={onSearch}
            />
          )}
          {rangeError && (
            <Alert variant='destructive'>
              <AlertDescription>{rangeError}</AlertDescription>
            </Alert>
          )}
          {tab === 'revenue' && (
            <RevenueTab
              range={range}
              model={filters.model_name}
              canQuery={rangeError === ''}
              canWrite={access.revenueWrite}
              canViewContent={access.content}
              breakdown={search.routeSearch.breakdown ?? 'rule'}
              onBreakdownChange={search.selectBreakdown}
              onFilter={search.range.updateFilters}
              onViewSessions={(model) => search.selectTab('content', model)}
            />
          )}
          {tab === 'content' && (
            <ContentTab
              filters={search.sessionFilters}
              canQuery={rangeError === ''}
              pagination={search.range.pagination}
              onPaginationChange={search.range.onPaginationChange}
            />
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

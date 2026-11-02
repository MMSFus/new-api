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
import type { PaginationState } from '@tanstack/react-table'

import {
  getRecordContent,
  getRevenueRules,
  getRevenueSummary,
  getSessionRecords,
  getSessions,
  type RevenueSummaryParams,
  type SessionFilters,
  type SessionSummary,
} from './api'

export const SESSION_RECORDER_QUERY_KEY = ['session-recorder-panel'] as const
export const REVENUE_RULES_QUERY_KEY = [
  ...SESSION_RECORDER_QUERY_KEY,
  'revenue-rules',
] as const
export const REVENUE_SUMMARY_QUERY_KEY = [
  ...SESSION_RECORDER_QUERY_KEY,
  'revenue-summary',
] as const

const EMPTY_SESSIONS: SessionSummary[] = []

export function useRevenueRules(enabled: boolean) {
  return useQuery({
    queryKey: REVENUE_RULES_QUERY_KEY,
    queryFn: getRevenueRules,
    enabled,
    retry: false,
  })
}

export function useRevenueSummary(
  params: RevenueSummaryParams,
  enabled: boolean
) {
  return useQuery({
    queryKey: [...REVENUE_SUMMARY_QUERY_KEY, params],
    queryFn: () => getRevenueSummary(params),
    enabled,
    retry: false,
  })
}

export function useSessions(
  filters: SessionFilters,
  pagination: PaginationState,
  enabled: boolean
) {
  const params = {
    ...filters,
    p: pagination.pageIndex + 1,
    page_size: pagination.pageSize,
  }
  const query = useQuery({
    queryKey: [...SESSION_RECORDER_QUERY_KEY, 'sessions', params],
    queryFn: () => getSessions(params),
    enabled,
    retry: false,
  })
  return {
    query,
    sessions: enabled && query.data ? query.data.items : EMPTY_SESSIONS,
    total: enabled && !query.isError ? (query.data?.total ?? 0) : 0,
  }
}

export function useSessionRecords(sessionId: string) {
  return useQuery({
    queryKey: [...SESSION_RECORDER_QUERY_KEY, 'records', sessionId],
    queryFn: () => getSessionRecords(sessionId),
    retry: false,
  })
}

export function useRecordContent(recordId: string) {
  return useQuery({
    queryKey: [...SESSION_RECORDER_QUERY_KEY, 'content', recordId],
    queryFn: () => getRecordContent(recordId),
    retry: false,
    // Content is large and every read is audited; do not refetch on focus.
    staleTime: 10 * 60 * 1000,
    refetchOnWindowFocus: false,
  })
}

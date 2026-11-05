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
import { t } from 'i18next'

import type { ApiResponse } from '@/features/profile/types'
import { api } from '@/lib/api'
import { createServerError } from '@/lib/server-error-message'

const BASE = '/api/session_recorder'

export const REVENUE_MATCH_TYPES = ['keyword', 'regex', 'glob'] as const
export type RevenueMatchType = (typeof REVENUE_MATCH_TYPES)[number]

/** One revenue rule; the array order is the match priority. */
export interface RevenueRule {
  id: string
  name: string
  match_type: RevenueMatchType
  pattern: string
  /** Revenue per 10^12 compressed bytes, in the display currency. */
  price_per_tb: number
  enabled: boolean
}

export interface RevenueRuleSet {
  rules: RevenueRule[]
  /** Display-only currency label; empty means the site currency symbol. */
  currency_label: string
}

export interface RevenueRuleTestResult {
  matched_rule_id: string | null
  matched_rule_name: string | null
}

export type RevenueGranularity = 'hour' | 'day'

export interface RevenueSummaryParams {
  start_timestamp: number
  end_timestamp: number
  model?: string
  granularity: RevenueGranularity
}

export interface RevenueTotals {
  records: number
  raw_bytes: number
  archive_bytes: number
  revenue: number
  unarchived_records: number
  unarchived_raw_bytes: number
}

/** Every enabled rule in priority order, matched or not; never disabled ones. */
export interface RevenueByRule {
  rule_id: string
  rule_name: string
  price_per_tb: number
  records: number
  archive_bytes: number
  revenue: number
}

/** Sorted by revenue and archive bytes descending, then model name. */
export interface RevenueByModel {
  model_name: string
  /** Null when no enabled rule matches the model. */
  rule_id: string | null
  records: number
  raw_bytes: number
  archive_bytes: number
  revenue: number
}

export interface RevenueBucket {
  /**
   * Start of the bucket, Unix seconds, aligned to the UTC hour or UTC
   * midnight. Only buckets with data are returned.
   */
  bucket: number
  archive_bytes: number
  revenue: number
}

export interface RevenueSummary {
  totals: RevenueTotals
  by_rule: RevenueByRule[] | null
  unmatched: { records: number; archive_bytes: number }
  by_model: RevenueByModel[] | null
  trend: RevenueBucket[] | null
}

export interface SessionFilters {
  start_timestamp?: number
  end_timestamp?: number
  model?: string
  user_id?: number
  session_id?: string
}

export interface SessionSummary {
  session_id: string
  first_at: number
  last_at: number
  records: number
  models: string[] | null
  user_ids: number[] | null
  raw_bytes: number
  archive_bytes: number
}

export interface SessionPage {
  items: SessionSummary[]
  total: number
}

/** Index row of one recorded request. */
export interface SessionRecord {
  record_id: string
  session_id: string
  created_at: number
  duration_ms: number
  model_name: string
  group: string
  channel_id: number
  user_id: number
  token_id: number
  status_code: number
  stream: boolean
  path: string
  raw_bytes: number
  archive_bytes: number
  batch_id: string
  object_key: string
  root_id: string
  record_path: string
  archived_at: number
}

/** A session's records in time order; at most `limit` of `total`. */
export interface SessionRecordList {
  items: SessionRecord[]
  total: number
  limit: number
}

export interface RecordBody {
  content_type: string
  /** Full size in bytes before truncation. */
  size: number
  truncated: boolean
  encoding: 'utf8' | 'base64'
  text: string
}

export interface RecordContent {
  /** The recorder's Record JSON, passed through unchanged. */
  record: Record<string, unknown>
  request: RecordBody | null
  response: RecordBody | null
  source: 'local' | 'archive'
}

export type BackfillState = 'idle' | 'running' | 'done' | 'failed' | 'cancelled'

export interface BackfillStatus {
  /** `running` until the current batch ends, even after a cancel request. */
  state: BackfillState
  /** Empty while idle. */
  task_id: string
  /** 0–100. */
  progress: number
  total_batches: number
  processed_batches: number
  indexed_records: number
  skipped_batches: number
  /** Retried on the next run; a run with failures can still be `done`. */
  failed_batches: number
  last_error: string
  started_at: number
  finished_at: number
}

async function unwrap<T>(
  request: Promise<{ data: ApiResponse<T> }>,
  fallback: string
): Promise<T> {
  const response = await request
  if (!response.data.success || response.data.data === undefined) {
    throw createServerError(response.data, fallback)
  }
  return response.data.data
}

export function getRevenueRules(): Promise<RevenueRuleSet> {
  return unwrap(
    api.get<ApiResponse<RevenueRuleSet>>(`${BASE}/revenue/rules`),
    t('Failed to load revenue rules')
  )
}

/** Returns the canonical rule snapshot stored by the server. */
export function saveRevenueRules(
  body: RevenueRuleSet
): Promise<RevenueRuleSet> {
  return unwrap(
    api.put<ApiResponse<RevenueRuleSet>>(`${BASE}/revenue/rules`, body),
    t('Failed to save revenue rules')
  )
}

export function testRevenueRules(body: {
  rules: RevenueRule[]
  model: string
}): Promise<RevenueRuleTestResult> {
  return unwrap(
    api.post<ApiResponse<RevenueRuleTestResult>>(
      `${BASE}/revenue/rules/test`,
      body
    ),
    t('Failed to test the model name')
  )
}

export function getRevenueSummary(
  params: RevenueSummaryParams
): Promise<RevenueSummary> {
  return unwrap(
    api.get<ApiResponse<RevenueSummary>>(`${BASE}/revenue/summary`, {
      params,
    }),
    t('Failed to load revenue summary')
  )
}

export function getSessions(
  params: SessionFilters & { p: number; page_size: number }
): Promise<SessionPage> {
  return unwrap(
    api.get<ApiResponse<SessionPage>>(`${BASE}/sessions`, { params }),
    t('Failed to load sessions')
  )
}

export function getSessionRecords(
  sessionId: string
): Promise<SessionRecordList> {
  // A query parameter, since session ids may contain "/".
  return unwrap(
    api.get<ApiResponse<SessionRecordList>>(`${BASE}/sessions/records`, {
      params: { session_id: sessionId },
    }),
    t('Failed to load session records')
  )
}

export function getRecordContent(recordId: string): Promise<RecordContent> {
  return unwrap(
    api.get<ApiResponse<RecordContent>>(
      `${BASE}/records/${encodeURIComponent(recordId)}/content`
    ),
    t('Failed to load record content')
  )
}

export type RecordBodyKind = 'request' | 'response'

/** The complete stored body, past the 2 MiB of the content API. */
export interface FullRecordBody {
  contentType: string
  bytes: Uint8Array<ArrayBuffer>
  /** The recorder itself cut the body at its size limit. */
  truncated: boolean
}

/**
 * Downloads one stored body. A success carries `X-Recorder-Truncated`; an
 * error is the usual JSON envelope, which arrives as a blob here. The caller
 * falls back to the shown part and says so, so no error toast is raised.
 */
export async function getRecordBody(
  recordId: string,
  kind: RecordBodyKind
): Promise<FullRecordBody> {
  const fallback = t('Failed to load the full body')
  const response = await api.get<Blob>(
    `${BASE}/records/${encodeURIComponent(recordId)}/body`,
    { params: { kind }, responseType: 'blob', skipErrorHandler: true }
  )
  const header = response.headers['x-recorder-truncated']
  const bytes = new Uint8Array(await response.data.arrayBuffer())
  if (header === undefined) {
    let envelope: unknown
    try {
      envelope = JSON.parse(new TextDecoder().decode(bytes))
    } catch {
      envelope = undefined
    }
    throw createServerError(envelope, fallback)
  }
  return {
    contentType: String(response.headers['content-type'] ?? ''),
    bytes,
    truncated: header === 'true',
  }
}

export function getBackfillStatus(): Promise<BackfillStatus> {
  return unwrap(
    api.get<ApiResponse<BackfillStatus>>(`${BASE}/backfill`),
    t('Failed to load backfill status')
  )
}

export function startBackfill(): Promise<BackfillStatus> {
  return unwrap(
    api.post<ApiResponse<BackfillStatus>>(`${BASE}/backfill`),
    t('Failed to start index backfill')
  )
}

/** Asks the run to stop after its current batch; returns the status. */
export function cancelBackfill(): Promise<BackfillStatus> {
  return unwrap(
    api.delete<ApiResponse<BackfillStatus>>(`${BASE}/backfill`),
    t('Failed to cancel index backfill')
  )
}

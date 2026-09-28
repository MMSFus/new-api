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

export type SessionRecorderFilterRule = {
  channel_id?: number
  group?: string
  model?: string
}

export type SessionRecorderFilter = {
  include: SessionRecorderFilterRule[] | null
  exclude: SessionRecorderFilterRule[] | null
}

/** Mirrors pkg/sessionrecorder.Settings. Credentials are never included. */
export type SessionRecorderSettings = {
  enabled: boolean
  producer_enabled: boolean
  consumer_enabled: boolean
  spool_root: string
  root_id: string
  queue_size: number
  max_body_bytes: number
  record_request_body: boolean
  record_response_body: boolean
  stats_path: string
  filter: SessionRecorderFilter
  upload_enabled: boolean
  local_cleanup_enabled: boolean
  installation_id: string
  site_id: string
  host_id: string
  work_dir: string
  r2_endpoint: string
  r2_region: string
  r2_bucket: string
  r2_object_prefix: string
  credentials_dir: string
  scan_interval_seconds: number
  stability_window_seconds: number
  max_batch_files: number
  max_batch_input_bytes: number
  target_archive_bytes: number
  gzip_level: number
  multipart_concurrency: number
  max_upload_mbps: number
  adaptive_upload: boolean
  max_retries: number
  retry_initial_backoff_seconds: number
  retry_max_backoff_seconds: number
  busy_threshold_percent: number
  freshness_seconds: number
  external_stats_path: string
}

export type CredentialState = 'missing' | 'invalid' | 'configured'

export type SessionRecorderProducerCounters = {
  enqueued: number
  written: number
  dropped_queue_full: number
  dropped_memory_budget: number
  dropped_storage: number
  write_errors: number
  queue_length: number
  queue_capacity: number
  inflight_bytes: number
  usage_percent: number
  storage_paused: boolean
  last_error?: string
}

export type SessionRecorderReport = {
  setup_completed: boolean
  upload_enabled: boolean
  local_cleanup_enabled: boolean
  credential_state: CredentialState
  queue_state: string
  active_batch_id?: string
  active_batch_state?: string
  backlog_files: number
  backlog_bytes: number
  orphan_media: number
  issues: number
  disk_usage_percent: number
  inode_usage_percent: number
}

export type SessionRecorderStatus = {
  enabled: boolean
  producer_running: boolean
  consumer_running: boolean
  consumer_state: string
  credential_state: CredentialState
  settings_error?: string
  producer?: SessionRecorderProducerCounters
  report?: SessionRecorderReport
  last_cycle?: {
    at: string
    state: string
    batch_id?: string
    scanned_files: number
    orphan_media: number
    issues: number
  }
  last_error?: string
  last_upload_mbps: number
  recent_issues: Array<{ path: string; category: string }>
}

export type SessionRecorderCheckResult = {
  credential_state: CredentialState
  state: 'ok' | 'failed'
  error?: string
}

export type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data?: T
}

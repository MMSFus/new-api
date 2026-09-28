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
import { z } from 'zod'

import type {
  SessionRecorderFilterRule,
  SessionRecorderSettings,
} from '../types'

const int = (min: number, max = Number.MAX_SAFE_INTEGER) =>
  z.number().int().min(min).max(max)

const ruleSchema = z.object({
  channel_id: int(0),
  group: z.string(),
  model: z.string(),
})

export const recorderSettingsSchema = z.object({
  enabled: z.boolean(),
  producer_enabled: z.boolean(),
  consumer_enabled: z.boolean(),
  spool_root: z.string(),
  root_id: z.string(),
  queue_size: int(1, 1 << 20),
  max_body_bytes: int(0),
  record_request_body: z.boolean(),
  record_response_body: z.boolean(),
  stats_path: z.string(),
  filter: z.object({
    include: z.array(ruleSchema),
    exclude: z.array(ruleSchema),
  }),
  upload_enabled: z.boolean(),
  local_cleanup_enabled: z.boolean(),
  installation_id: z.string(),
  site_id: z.string(),
  host_id: z.string(),
  work_dir: z.string(),
  r2_endpoint: z.string(),
  r2_region: z.string(),
  r2_bucket: z.string(),
  r2_object_prefix: z.string(),
  credentials_dir: z.string(),
  scan_interval_seconds: int(5),
  stability_window_seconds: int(1),
  max_batch_files: int(1),
  max_batch_input_bytes: int(1),
  target_archive_bytes: int(1),
  gzip_level: int(1, 9),
  multipart_concurrency: int(1, 64),
  max_upload_mbps: int(0),
  adaptive_upload: z.boolean(),
  max_retries: int(0),
  retry_initial_backoff_seconds: int(1),
  retry_max_backoff_seconds: int(1),
  busy_threshold_percent: int(1, 100),
  freshness_seconds: int(1),
  external_stats_path: z.string(),
})

export type RecorderFormValues = z.infer<typeof recorderSettingsSchema>
export type RecorderRuleFormValue = z.infer<typeof ruleSchema>

function toFormRule(rule: SessionRecorderFilterRule): RecorderRuleFormValue {
  return {
    channel_id: rule.channel_id ?? 0,
    group: rule.group ?? '',
    model: rule.model ?? '',
  }
}

function toApiRule(rule: RecorderRuleFormValue): SessionRecorderFilterRule {
  const out: SessionRecorderFilterRule = {}
  if (rule.channel_id > 0) out.channel_id = rule.channel_id
  if (rule.group.trim()) out.group = rule.group.trim()
  if (rule.model.trim()) out.model = rule.model.trim()
  return out
}

export function toFormValues(
  settings: SessionRecorderSettings
): RecorderFormValues {
  return {
    ...settings,
    filter: {
      include: (settings.filter?.include ?? []).map(toFormRule),
      exclude: (settings.filter?.exclude ?? []).map(toFormRule),
    },
  }
}

export function toApiSettings(
  values: RecorderFormValues
): SessionRecorderSettings {
  return {
    ...values,
    filter: {
      include: values.filter.include.map(toApiRule),
      exclude: values.filter.exclude.map(toApiRule),
    },
  }
}

export const EMPTY_RULE: RecorderRuleFormValue = {
  channel_id: 0,
  group: '',
  model: '',
}

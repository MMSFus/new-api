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
import { RefreshCw, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { useCheckSessionRecorder, useSessionRecorderStatus } from './api'
import type { CredentialState } from './types'

type StatusPanelProps = {
  enabled: boolean
}

type StatItem = {
  label: string
  value: number | string
}

function credentialVariant(
  state: CredentialState | undefined
): StatusBadgeProps['variant'] {
  if (state === 'configured') return 'success'
  if (state === 'invalid') return 'danger'
  return 'warning'
}

function runningVariant(running: boolean): StatusBadgeProps['variant'] {
  return running ? 'success' : 'neutral'
}

export function SessionRecorderStatusPanel(props: StatusPanelProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const statusQuery = useSessionRecorderStatus(props.enabled)
  const check = useCheckSessionRecorder()
  const status = statusQuery.data
  const producer = status?.producer
  const report = status?.report

  const credentialLabel = (state: CredentialState | undefined) => {
    if (state === 'configured') return t('Configured')
    if (state === 'invalid') return t('Invalid')
    return t('Missing')
  }
  const num = (value: number | undefined) => formatNumber(value ?? 0, locale)
  const stats: StatItem[] = []
  if (producer) {
    stats.push(
      { label: t('Recorded'), value: num(producer.written) },
      { label: t('Queue usage'), value: `${producer.usage_percent}%` },
      {
        label: t('Dropped (queue full)'),
        value: num(producer.dropped_queue_full),
      },
      {
        label: t('Dropped (memory budget)'),
        value: num(producer.dropped_memory_budget),
      },
      { label: t('Dropped (disk full)'), value: num(producer.dropped_storage) },
      { label: t('Write errors'), value: num(producer.write_errors) }
    )
  }
  if (report) {
    stats.push(
      { label: t('Backlog files'), value: num(report.backlog_files) },
      { label: t('Orphan media'), value: num(report.orphan_media) },
      { label: t('Scan issues'), value: num(report.issues) },
      { label: t('Disk usage'), value: `${report.disk_usage_percent}%` },
      { label: t('Inode usage'), value: `${report.inode_usage_percent}%` },
      {
        label: t('Upload speed (Mbps)'),
        value: status?.last_upload_mbps.toFixed(1) ?? '0',
      }
    )
  }

  return (
    <div className='space-y-3 rounded-lg border p-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex flex-wrap items-center gap-2'>
          <StatusBadge
            label={t('Producer')}
            variant={runningVariant(Boolean(status?.producer_running))}
          />
          <StatusBadge
            label={`${t('Uploader')}: ${status?.consumer_state ?? '-'}`}
            variant={runningVariant(Boolean(status?.consumer_running))}
          />
          <StatusBadge
            label={`${t('Credentials')}: ${credentialLabel(status?.credential_state)}`}
            variant={credentialVariant(status?.credential_state)}
          />
          {report?.active_batch_id && (
            <StatusBadge
              label={`${t('Active batch')}: ${report.active_batch_state ?? ''}`}
              variant='info'
            />
          )}
        </div>
        <div className='flex gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => statusQuery.refetch()}
            disabled={statusQuery.isFetching}
          >
            <RefreshCw className='size-4' />
            {t('Refresh')}
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => check.mutate()}
            disabled={check.isPending}
          >
            <ShieldCheck className='size-4' />
            {t('Check R2 connectivity')}
          </Button>
        </div>
      </div>

      {status?.settings_error && (
        <Alert variant='destructive'>
          <AlertDescription>
            {t('Saved settings are invalid: {{error}}', {
              error: status.settings_error,
            })}
          </AlertDescription>
        </Alert>
      )}
      {status?.last_error && (
        <p className='text-destructive text-xs break-all'>
          {t('Last error')}: {status.last_error}
        </p>
      )}

      {stats.length > 0 && (
        <div className='grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-6'>
          {stats.map((item) => (
            <div key={item.label} className='space-y-0.5'>
              <div className='text-muted-foreground text-xs'>{item.label}</div>
              <div className='text-sm font-medium tabular-nums'>
                {item.value}
              </div>
            </div>
          ))}
        </div>
      )}

      {status?.last_cycle && (
        <p className='text-muted-foreground text-xs'>
          {t('Last cycle')}: {status.last_cycle.at} · {status.last_cycle.state}
          {status.last_cycle.batch_id ? ` · ${status.last_cycle.batch_id}` : ''}
        </p>
      )}

      {(status?.recent_issues?.length ?? 0) > 0 && (
        <div className='space-y-1'>
          <div className='text-xs font-medium'>{t('Recent scan issues')}</div>
          <ul className='text-muted-foreground max-h-40 overflow-auto text-xs'>
            {status?.recent_issues.map((issue) => (
              <li key={`${issue.path}-${issue.category}`} className='break-all'>
                {issue.category}: {issue.path}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

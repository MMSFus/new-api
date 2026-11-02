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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { MetricGrid } from '@/components/metric-grid'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { translateServerText } from '@/lib/server-error-message'

import type { BackfillState } from '../api'
import { formatUnixTime } from '../lib'
import type { useBackfill } from './use-backfill'

const STATE_LABELS: Record<BackfillState, string> = {
  idle: 'Not started',
  running: 'Running',
  done: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
}

/** Status, progress and start/cancel of the historical index backfill. */
export function BackfillPanel(props: {
  backfill: ReturnType<typeof useBackfill>
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [confirming, setConfirming] = useState(false)
  const backfill = props.backfill
  const status = backfill.status
  const statusQuery = backfill.statusQuery
  const mutationError = backfill.start.error ?? backfill.cancel.error

  if (statusQuery.isError) {
    return (
      <Alert variant='destructive'>
        <AlertDescription className='flex items-center justify-between gap-3'>
          <span>{t('Failed to load backfill status')}</span>
          <Button
            size='sm'
            variant='outline'
            onClick={() => void statusQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  if (!status) {
    return <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
  }

  const partlyFailed = status.state === 'done' && status.failed_batches > 0
  const resumable =
    (status.state === 'cancelled' || status.state === 'failed') &&
    status.processed_batches > 0
  const busy = backfill.start.isPending || backfill.cancel.isPending
  let startLabel = t('Start backfill')
  if (partlyFailed) startLabel = t('Retry failed batches')
  else if (resumable) startLabel = t('Resume backfill')
  let stateLabel = t(STATE_LABELS[status.state])
  if (backfill.cancelling) stateLabel = t('Cancelling')
  else if (partlyFailed) stateLabel = t('Completed with failures')

  return (
    <div className='space-y-4'>
      <section aria-live='polite' className='space-y-3 rounded-xl border p-4'>
        <div className='flex items-center justify-between gap-2'>
          <h3 className='text-sm font-medium'>{t('Backfill status')}</h3>
          <Badge
            variant={
              status.state === 'failed' || partlyFailed
                ? 'destructive'
                : 'secondary'
            }
          >
            {stateLabel}
          </Badge>
        </div>
        {status.state !== 'idle' && (
          <Progress aria-label={t('Progress')} value={status.progress} />
        )}
        {backfill.cancelling && (
          <p className='text-muted-foreground text-sm'>
            {t(
              'Cancelling: the backfill stops after the current batch. Indexed batches are kept.'
            )}
          </p>
        )}
        {partlyFailed && (
          <p className='text-warning text-sm'>
            {t(
              '{{count}} batches failed. Run the backfill again to retry them.',
              { count: status.failed_batches }
            )}
          </p>
        )}
        <MetricGrid
          metrics={[
            {
              label: t('Batches'),
              value: `${formatNumber(status.processed_batches, locale)} / ${formatNumber(status.total_batches, locale)}`,
            },
            {
              label: t('Indexed records'),
              value: formatNumber(status.indexed_records, locale),
            },
            {
              label: t('Skipped batches'),
              value: formatNumber(status.skipped_batches, locale),
            },
            {
              label: t('Failed batches'),
              value: formatNumber(status.failed_batches, locale),
              className: status.failed_batches > 0 ? 'text-warning' : undefined,
            },
            {
              label: t('Started at'),
              value: formatUnixTime(status.started_at),
            },
            {
              label: t('Finished at'),
              value: formatUnixTime(status.finished_at),
            },
          ]}
        />
        {status.last_error && (
          <p className='text-destructive text-sm break-all'>
            {t('Last error')}: {translateServerText(t, status.last_error)}
          </p>
        )}
      </section>
      {mutationError && (
        <p role='alert' className='text-destructive text-sm'>
          {translateServerText(t, mutationError.message)}
        </p>
      )}
      <div className='flex flex-wrap justify-end gap-2'>
        {backfill.running ? (
          <Button
            type='button'
            variant='outline'
            disabled={busy || backfill.cancelling}
            onClick={() => {
              backfill.start.reset()
              backfill.cancel.mutate()
            }}
          >
            {backfill.cancelling ? t('Cancelling') : t('Cancel backfill')}
          </Button>
        ) : (
          <Button
            type='button'
            disabled={busy}
            onClick={() => {
              backfill.cancel.reset()
              setConfirming(true)
            }}
          >
            {startLabel}
          </Button>
        )}
      </div>
      <ConfirmDialog
        open={confirming}
        onOpenChange={(open) => {
          if (!open) setConfirming(false)
        }}
        title={t('Backfill the recording index')}
        confirmText={t('Start background backfill')}
        isLoading={backfill.start.isPending}
        handleConfirm={() => {
          setConfirming(false)
          backfill.start.mutate()
        }}
        desc={
          <div className='space-y-3 text-sm leading-6'>
            <p>
              {t(
                'Downloads every uploaded archive from R2 and indexes its records, so revenue and conversation review cover data recorded before this page existed.'
              )}
            </p>
            <p>
              {t(
                'It runs in the background on the master node without blocking recording. A cancelled run resumes from the batches it has not finished.'
              )}
            </p>
          </div>
        }
      />
    </div>
  )
}

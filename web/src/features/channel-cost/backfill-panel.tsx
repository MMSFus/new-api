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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import {
  startCostBackfill,
  type ChannelCostFilters,
  type CostBackfillStatus,
} from './api'
import { CostBackfillTaskStatus } from './backfill-task'
import { formatBackfillRange } from './lib'
import {
  COST_BACKFILL_QUERY_KEY,
  useCostBackfillStatus,
} from './use-cost-backfill'

type BackfillPanelProps = {
  filters: ChannelCostFilters
  missing?: number
  enabled: boolean
  /**
   * Refresh the report when the task finishes. Off when a parent that stays
   * mounted (the header entry) already does it.
   */
  trackCompletion?: boolean
}

export function ChannelCostBackfillPanel(props: BackfillPanelProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [confirmation, setConfirmation] = useState<{
    filters: ChannelCostFilters
    missing: number
  } | null>(null)
  const [existingTask, setExistingTask] = useState(false)
  const backfill = useCostBackfillStatus({
    trackCompletion: props.trackCompletion ?? true,
  })
  const statusQuery = backfill.statusQuery
  const mutation = useMutation({
    mutationFn: startCostBackfill,
    onSuccess: (data) => {
      queryClient.setQueryData<CostBackfillStatus>(COST_BACKFILL_QUERY_KEY, {
        supported: true,
        task: data.task,
      })
      setConfirmation(null)
      setExistingTask(!data.created)
    },
  })

  if (!backfill.isRoot) return null

  const missing = props.missing ?? 0
  const canStart =
    props.enabled &&
    missing > 0 &&
    !backfill.active &&
    statusQuery.data?.supported === true &&
    !statusQuery.isError

  return (
    <div className='space-y-4'>
      {statusQuery.isError && (
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
      )}
      {statusQuery.data?.supported === false && (
        <Alert>
          <AlertDescription>
            {t(
              'Historical cost backfill is not supported by this log database'
            )}
          </AlertDescription>
        </Alert>
      )}
      {missing > 0 && (
        <div className='rounded-xl border border-amber-500/30 bg-amber-500/5 p-5'>
          <div className='flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between'>
            <div>
              <h2 className='text-sm font-medium'>
                {t('The current range has {{number}} uncosted requests', {
                  number: formatNumber(missing, locale),
                })}
              </h2>
              <p className='text-muted-foreground mt-1 text-sm leading-6'>
                {t(
                  'Backfill historical costs using current channel ratios. Eligible records will then count toward cost and profit.'
                )}
              </p>
            </div>
            <Button
              className='shrink-0'
              disabled={!canStart || mutation.isPending}
              onClick={() => {
                mutation.reset()
                setConfirmation({ filters: { ...props.filters }, missing })
              }}
            >
              {t('Backfill historical costs')}
            </Button>
          </div>
        </div>
      )}
      {existingTask && (
        <p role='status' className='text-muted-foreground text-sm'>
          {t('A backfill task is already active. Showing its range.')}
        </p>
      )}
      {backfill.task && <CostBackfillTaskStatus task={backfill.task} />}
      <ConfirmDialog
        open={confirmation !== null}
        onOpenChange={(open) => {
          if (!open && !mutation.isPending) setConfirmation(null)
        }}
        title={t('Backfill historical costs')}
        confirmText={t('Start background backfill')}
        isLoading={mutation.isPending}
        disabled={!canStart}
        handleConfirm={() => {
          if (confirmation && canStart) mutation.mutate(confirmation.filters)
        }}
        desc={
          <div className='space-y-4 text-sm leading-6'>
            <p>
              {t(
                'This will backfill historical records with missing costs within the selected filters.'
              )}
            </p>
            <div className='bg-muted/30 rounded-lg border px-4 py-3'>
              <div className='text-foreground font-medium'>
                {t('Statistics period')}
              </div>
              <div className='text-xs tabular-nums'>
                {formatBackfillRange(
                  confirmation?.filters.start_timestamp,
                  confirmation?.filters.end_timestamp
                )}
              </div>
              <div className='mt-2'>
                {t('Uncosted requests')}:{' '}
                {formatNumber(confirmation?.missing ?? 0, locale)}
              </div>
              <div className='text-xs'>
                {t(
                  'Includes the current channel, model, group and user filters.'
                )}
              </div>
            </div>
            <p>
              {t(
                'Costs are estimated using channel ratios at task start and billing information saved in the logs. Results are marked as historical backfill and may differ from past upstream bills.'
              )}
            </p>
            <p>
              {t(
                'Only missing costs are filled. Existing costs, user charges and balances stay unchanged. Records that cannot be reliably calculated are skipped, with reasons shown after completion.'
              )}
            </p>
            <p>
              {t(
                'The task runs in batches in the background and continues after you close this page.'
              )}
            </p>
            {mutation.isError && (
              <p role='alert' className='text-destructive'>
                {t(mutation.error.message)}
              </p>
            )}
          </div>
        }
      />
    </div>
  )
}

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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { toIntlLocale } from '@/i18n/languages'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getCostBackfillStatus,
  startCostBackfill,
  type ChannelCostFilters,
  type CostBackfillStatus,
} from './api'

type BackfillPanelProps = {
  filters: ChannelCostFilters
  missing?: number
  enabled: boolean
}

const taskTitles = {
  pending: 'Historical cost backfill submitted',
  running: 'Backfilling historical costs',
  succeeded: 'Historical cost backfill complete',
  failed: 'Historical cost backfill incomplete',
} as const
const taskStatuses = {
  pending: 'Pending',
  running: 'Running',
  succeeded: 'Completed',
  failed: 'Failed',
} as const

function formatRange(start?: number, end?: number): string {
  return `${dayjs.unix(start ?? 0).format('YYYY-MM-DD HH:mm')} — ${dayjs.unix(end ?? 0).format('YYYY-MM-DD HH:mm')}`
}

export function ChannelCostBackfillPanel(props: BackfillPanelProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const queryClient = useQueryClient()
  const [confirmation, setConfirmation] = useState<{
    filters: ChannelCostFilters
    missing: number
  } | null>(null)
  const [existingTask, setExistingTask] = useState(false)
  const lastTerminalTask = useRef('')
  const statusQuery = useQuery({
    queryKey: ['channel-cost-backfill'],
    queryFn: getCostBackfillStatus,
    enabled: isRoot,
    retry: false,
    refetchInterval: (query) =>
      ['pending', 'running'].includes(query.state.data?.task?.status ?? '')
        ? 3000
        : 15000,
    refetchIntervalInBackground: false,
    meta: { errorToast: false },
  })
  const task = statusQuery.data?.task
  const active = task?.status === 'pending' || task?.status === 'running'
  const scope = task?.payload?.filter
  const state = task?.result ?? task?.state
  const mutation = useMutation({
    mutationFn: startCostBackfill,
    onSuccess: (data) => {
      queryClient.setQueryData<CostBackfillStatus>(['channel-cost-backfill'], {
        supported: true,
        task: data.task,
      })
      setConfirmation(null)
      setExistingTask(!data.created)
    },
  })

  useEffect(() => {
    if (!task || active || lastTerminalTask.current === task.task_id) return
    lastTerminalTask.current = task.task_id
    void queryClient.invalidateQueries({ queryKey: ['channel-cost'] })
  }, [task, active, queryClient])

  if (!isRoot) return null

  const canStart =
    props.enabled &&
    (props.missing ?? 0) > 0 &&
    !active &&
    statusQuery.data?.supported === true &&
    !statusQuery.isError
  const taskFilters = scope
    ? [
        scope.Channel ? `${t('Channel')} #${scope.Channel}` : '',
        scope.ModelName ? `${t('Model')}: ${scope.ModelName}` : '',
        scope.Username ? `${t('Username')}: ${scope.Username}` : '',
        scope.Group ? `${t('Group')}: ${scope.Group}` : '',
      ]
        .filter(Boolean)
        .join(' · ')
    : ''

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
      {(props.missing ?? 0) > 0 && (
        <div className='rounded-xl border border-amber-500/30 bg-amber-500/5 p-5'>
          <div className='flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between'>
            <div>
              <h2 className='text-sm font-medium'>
                {t('The current range has {{number}} uncosted requests', {
                  number: formatNumber(props.missing ?? 0, locale),
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
                setConfirmation({
                  filters: { ...props.filters },
                  missing: props.missing ?? 0,
                })
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
      {task && (
        <section aria-live='polite' className='space-y-4 rounded-xl border p-5'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div>
              <h2 className='text-sm font-medium'>
                {t(taskTitles[task.status])}
              </h2>
              {scope && (
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('Task range')}:{' '}
                  {formatRange(scope.StartTimestamp, scope.EndTimestamp)}
                </p>
              )}
              {taskFilters && (
                <p className='text-muted-foreground mt-1 text-xs break-all'>
                  {taskFilters}
                </p>
              )}
            </div>
            <Badge variant='secondary'>{t(taskStatuses[task.status])}</Badge>
          </div>
          {state && (
            <>
              <div className='flex flex-wrap gap-x-5 gap-y-2 text-sm'>
                <span>
                  {t('Scanned')}: {formatNumber(state.processed, locale)}
                </span>
                <span>
                  {t('Backfilled')}: {formatNumber(state.updated, locale)}
                </span>
                <span>
                  {t('Skipped')}: {formatNumber(state.skipped, locale)}
                </span>
              </div>
              {active && (
                <Progress
                  aria-label={t('Progress')}
                  value={state.progress ?? 0}
                />
              )}
            </>
          )}
          {active && (
            <p className='text-muted-foreground text-sm'>
              {t(
                'The task runs in batches in the background and continues after you close this page.'
              )}
            </p>
          )}
          {task.status === 'succeeded' && (
            <p className='text-muted-foreground text-sm'>
              {t(
                'Backfilled costs are estimates, marked as historical backfill in the details. The report has been refreshed.'
              )}
            </p>
          )}
          {task.status === 'failed' && (
            <>
              <p className='text-destructive text-sm'>
                {t('Historical cost backfill failed. Please retry later.')}
              </p>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Completed estimates are preserved. Retry processes records that are still uncosted.'
                )}
              </p>
            </>
          )}
          {!!state?.skipped && (
            <ul className='text-muted-foreground space-y-1 text-sm'>
              {Object.entries(state.reasons ?? {}).map(([reason, count]) => (
                <li key={reason}>
                  {t(reason)}: {formatNumber(count, locale)}
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
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
                {formatRange(
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

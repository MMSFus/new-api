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
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import {
  getUpstreamCostRatioHistory,
  probeUpstreamCostRatio,
  syncUpstreamCostRatio,
  type UpstreamCostRatioHistoryItem,
} from '../api'
import {
  channelsQueryKeys,
  formatRatio,
  UPSTREAM_RATIO_STATUS_LABELS as STATUS_LABELS,
  UPSTREAM_RATIO_STATUS_VARIANTS as STATUS_VARIANTS,
} from '../lib'

const upstreamCostRatioQueryKeys = {
  history: (channelId: number) =>
    [...channelsQueryKeys.detail(channelId), 'upstream_cost_ratio'] as const,
}

const HISTORY_PAGE_SIZE = 10

type UpstreamCostRatioPanelProps = {
  channelId: number
  /** Sync and probe are offered for saved channels on a custom upstream. */
  syncSupported: boolean
  disabled?: boolean
  /** Called with the ratio the user chose to fill into the form. */
  onApplyRatio: (ratio: number) => void
}

export function UpstreamCostRatioPanel({
  channelId,
  syncSupported,
  disabled = false,
  onApplyRatio,
}: UpstreamCostRatioPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const historyQuery = useQuery({
    queryKey: upstreamCostRatioQueryKeys.history(channelId),
    queryFn: () =>
      getUpstreamCostRatioHistory(channelId, 1, HISTORY_PAGE_SIZE),
  })

  const probeMutation = useMutation({
    mutationFn: () => probeUpstreamCostRatio(channelId),
    onSuccess: (probe) => {
      onApplyRatio(probe.resolved_rate_multiplier)
      toast.success(
        t('Upstream ratio {{ratio}} filled in; save to apply', {
          ratio: probe.resolved_rate_multiplier,
        })
      )
      if (!probe.in_range) {
        toast.warning(t('This value is outside the sync range'))
      }
    },
    onError: (error) =>
      handleServerError(error, t('Failed to query upstream ratio')),
  })

  const syncMutation = useMutation({
    mutationFn: () => syncUpstreamCostRatio(channelId),
    onSuccess: async (result) => {
      if (result.status === 'success' && result.new_ratio != null) {
        onApplyRatio(result.new_ratio)
        toast.success(
          t('Upstream ratio synced: {{old}} → {{next}}', {
            old: formatRatio(result.old_ratio),
            next: formatRatio(result.new_ratio),
          })
        )
      } else if (result.status === 'unchanged') {
        toast.success(t(STATUS_LABELS.unchanged))
      } else {
        toast.warning(
          result.error
            ? `${t(STATUS_LABELS[result.status])}: ${result.error}`
            : t(STATUS_LABELS[result.status])
        )
      }
      await queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.detail(channelId),
      })
      await queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.lists(),
      })
    },
    onError: (error) =>
      handleServerError(error, t('Failed to sync upstream ratio')),
  })

  const syncStatus = historyQuery.data?.sync_status ?? null
  const items = historyQuery.data?.items ?? []
  const busy = probeMutation.isPending || syncMutation.isPending

  return (
    <div className='space-y-3 rounded-lg border p-3'>
      {syncSupported && (
        <div className='flex flex-wrap items-center gap-2'>
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={disabled || busy}
            onClick={() => probeMutation.mutate()}
          >
            {probeMutation.isPending && (
              <Loader2 className='animate-spin' aria-hidden='true' />
            )}
            {t('Fetch from upstream')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={disabled || busy}
            onClick={() => syncMutation.mutate()}
          >
            {syncMutation.isPending && (
              <Loader2 className='animate-spin' aria-hidden='true' />
            )}
            {t('Sync now')}
          </Button>
          {syncStatus && (
            <span className='text-muted-foreground flex flex-wrap items-center gap-2 text-xs'>
              <Badge variant={STATUS_VARIANTS[syncStatus.status]}>
                {t(STATUS_LABELS[syncStatus.status])}
              </Badge>
              {t('Last attempt')}:{' '}
              {formatTimestampToDate(syncStatus.last_attempt_at)}
              {syncStatus.error && (
                <span className='text-destructive break-all'>
                  {syncStatus.error}
                </span>
              )}
            </span>
          )}
        </div>
      )}
      <UpstreamCostRatioHistoryList
        items={items}
        total={historyQuery.data?.total ?? 0}
        loading={historyQuery.isLoading}
      />
    </div>
  )
}

function UpstreamCostRatioHistoryList({
  items,
  total,
  loading,
}: {
  items: UpstreamCostRatioHistoryItem[]
  total: number
  loading: boolean
}) {
  const { t } = useTranslation()

  if (loading) {
    return (
      <p className='text-muted-foreground text-xs'>{t('Loading...')}</p>
    )
  }
  if (items.length === 0) {
    return (
      <p className='text-muted-foreground text-xs'>
        {t('No ratio changes recorded yet')}
      </p>
    )
  }
  return (
    <div className='space-y-1'>
      <p className='text-xs font-medium'>{t('Ratio history')}</p>
      <ul aria-label={t('Ratio history')} className='space-y-1 text-xs'>
        {items.map((item) => (
          <li
            key={item.id}
            className='text-muted-foreground flex flex-wrap items-center gap-x-2'
          >
            <span className='tabular-nums'>
              {formatTimestampToDate(item.created_at)}
            </span>
            <span className='text-foreground tabular-nums'>
              {formatRatio(item.old_ratio)} → {formatRatio(item.new_ratio)}
            </span>
            <span>{item.currency || t('Site currency')}</span>
            <Badge variant='secondary'>
              {item.source === 'sync' ? t('Auto sync') : t('Manual')}
            </Badge>
          </li>
        ))}
      </ul>
      {total > items.length && (
        <p className='text-muted-foreground text-xs'>
          {t('Showing latest {{count}} of {{total}}', {
            count: items.length,
            total,
          })}
        </p>
      )}
    </div>
  )
}

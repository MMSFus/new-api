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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'

import {
  batchEnableUpstreamCostRatioSync,
  batchProbeUpstreamCostRatio,
  type UpstreamRatioBatchEnableRequest,
  type UpstreamRatioBatchProbeStatus,
} from '../../api'
import { channelsQueryKeys, formatRatio } from '../../lib'
import { UpstreamRatioBatchSummaryList } from './upstream-ratio-batch-summary'

const PROBE_LABELS: Record<UpstreamRatioBatchProbeStatus, string> = {
  supported: 'Supported',
  unsupported: 'Upstream does not support ratio query',
  ineligible: 'Not applicable',
  failed: 'Query failed',
  not_found: 'Channel not found',
}

const PROBE_VARIANTS: Record<
  UpstreamRatioBatchProbeStatus,
  'secondary' | 'warning' | 'destructive' | 'outline'
> = {
  supported: 'secondary',
  unsupported: 'outline',
  ineligible: 'outline',
  failed: 'destructive',
  not_found: 'outline',
}

/** Select value standing for '' (site currency). */
const SITE_CURRENCY = 'site'

/** Parses an optional 0-100 bound; undefined when invalid. */
function parseBound(text: string): number | null | undefined {
  if (text.trim() === '') return null
  const value = Number(text)
  return Number.isFinite(value) && value >= 0 && value <= 100
    ? value
    : undefined
}

type UpstreamRatioBatchDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelIds: number[]
  onDone: () => void
}

export function UpstreamRatioBatchDialog(props: UpstreamRatioBatchDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<Set<number> | null>(null)
  const [currency, setCurrency] =
    useState<UpstreamRatioBatchEnableRequest['currency']>('USD')
  const [syncMin, setSyncMin] = useState('')
  const [syncMax, setSyncMax] = useState('')
  const [syncNow, setSyncNow] = useState(true)

  const probeQuery = useQuery({
    queryKey: [...channelsQueryKeys.all, 'upstream_ratio_probe', props.channelIds],
    queryFn: () => batchProbeUpstreamCostRatio(props.channelIds),
    enabled: props.open && props.channelIds.length > 0,
    staleTime: 0,
    gcTime: 0,
  })
  const items = probeQuery.data ?? []
  // Supported rows start checked; the user can adjust before enabling.
  const checked =
    selected ??
    new Set(
      items
        .filter((item) => item.status === 'supported')
        .map((item) => item.channel_id)
    )

  const enableMutation = useMutation({
    mutationFn: batchEnableUpstreamCostRatioSync,
    onSuccess: async () => {
      toast.success(t('Auto sync enabled for selected channels'))
      await queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })
    },
    onError: (error) =>
      handleServerError(error, t('Failed to enable upstream ratio sync')),
  })

  const min = parseBound(syncMin)
  const max = parseBound(syncMax)
  const rangeInvalid =
    min === undefined ||
    max === undefined ||
    (min !== null && max !== null && min > max)

  const close = (open: boolean) => {
    if (!open) {
      setSelected(null)
      enableMutation.reset()
      if (enableMutation.isSuccess) props.onDone()
    }
    props.onOpenChange(open)
  }

  const toggle = (id: number, value: boolean) => {
    const next = new Set(checked)
    if (value) next.add(id)
    else next.delete(id)
    setSelected(next)
  }

  const submit = () => {
    if (rangeInvalid || checked.size === 0) return
    enableMutation.mutate({
      ids: [...checked],
      currency,
      sync_min: min ?? null,
      sync_max: max ?? null,
      sync_now: syncNow,
    })
  }

  const result = enableMutation.data

  let body: ReactNode
  if (result) {
    body = (
      <UpstreamRatioBatchSummaryList summary={result.summary} />
    )
  } else if (probeQuery.isPending) {
    body = (
      <p
        role='status'
        className='text-muted-foreground flex items-center gap-2 text-sm'
      >
        <Loader2 className='size-4 animate-spin' aria-hidden='true' />
        {t('Querying {{count}} channels...', {
          count: props.channelIds.length,
        })}
      </p>
    )
  } else if (probeQuery.isError) {
    body = (
      <p role='alert' className='text-destructive text-sm'>
        {t('Failed to query upstream ratio')}
      </p>
    )
  } else {
    body = (
      <>
        <ul
          aria-label={t('Detection results')}
          className='max-h-80 divide-y overflow-y-auto rounded-md border'
        >
          {items.map((item) => {
            const selectable = item.status === 'supported'
            const inputId = `upstream-ratio-batch-${item.channel_id}`
            return (
              <li
                key={item.channel_id}
                className='flex items-center gap-3 px-3 py-2 text-sm'
              >
                <Checkbox
                  id={inputId}
                  disabled={!selectable}
                  checked={checked.has(item.channel_id)}
                  onCheckedChange={(value) =>
                    toggle(item.channel_id, value === true)
                  }
                />
                <Label
                  htmlFor={inputId}
                  className='min-w-0 flex-1 truncate font-normal'
                >
                  #{item.channel_id} {item.name}
                </Label>
                <Badge variant={PROBE_VARIANTS[item.status]}>
                  {t(PROBE_LABELS[item.status])}
                </Badge>
                <span className='w-36 shrink-0 text-right font-mono tabular-nums'>
                  {selectable
                    ? `${formatRatio(item.current_ratio)} → ${formatRatio(item.resolved_rate_multiplier)}`
                    : ''}
                </span>
                {item.error && (
                  <span
                    className='text-muted-foreground max-w-48 truncate text-xs'
                    title={t(item.error)}
                  >
                    {t(item.error)}
                  </span>
                )}
              </li>
            )
          })}
        </ul>

        <div className='grid gap-3 sm:grid-cols-3'>
          <div className='space-y-1.5'>
            <Label>{t('Upstream ratio currency')}</Label>
            <Select
              value={currency || SITE_CURRENCY}
              onValueChange={(value) =>
                setCurrency(
                  value === SITE_CURRENCY
                    ? ''
                    : (value as UpstreamRatioBatchEnableRequest['currency'])
                )
              }
            >
              <SelectTrigger
                className='w-full'
                aria-label={t('Upstream ratio currency')}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value={SITE_CURRENCY}>
                    {t('Site currency')}
                  </SelectItem>
                  <SelectItem value='USD'>USD</SelectItem>
                  <SelectItem value='CNY'>CNY</SelectItem>
                  <SelectItem value='CUSTOM'>
                    {t('Custom currency')}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='upstream-ratio-batch-min'>
              {t('Sync range minimum')}
            </Label>
            <Input
              id='upstream-ratio-batch-min'
              type='number'
              inputMode='decimal'
              step='any'
              placeholder={t('No limit')}
              value={syncMin}
              aria-invalid={min === undefined}
              onChange={(e) => setSyncMin(e.target.value)}
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='upstream-ratio-batch-max'>
              {t('Sync range maximum')}
            </Label>
            <Input
              id='upstream-ratio-batch-max'
              type='number'
              inputMode='decimal'
              step='any'
              placeholder={t('No limit')}
              value={syncMax}
              aria-invalid={max === undefined || rangeInvalid}
              onChange={(e) => setSyncMax(e.target.value)}
            />
          </div>
        </div>
        {rangeInvalid && (
          <p role='alert' className='text-destructive text-xs'>
            {t('Sync range must be 0-100 and the minimum must not exceed the maximum')}
          </p>
        )}
        <div className='flex items-center justify-between gap-4'>
          <Label htmlFor='upstream-ratio-batch-now'>
            {t('Sync once now after enabling')}
          </Label>
          <Switch
            id='upstream-ratio-batch-now'
            checked={syncNow}
            onCheckedChange={setSyncNow}
          />
        </div>
      </>
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={close}
      title={t('Detect upstream ratio')}
      description={t(
        'Query /v1/sub2api/billing on each selected channel. Channels that answer can enable auto sync in one step.'
      )}
      contentClassName='sm:max-w-3xl'
      bodyClassName='space-y-4'
      footer={
        result ? (
          <Button onClick={() => close(false)}>{t('Done')}</Button>
        ) : (
          <>
            <Button variant='outline' onClick={() => close(false)}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={submit}
              disabled={
                probeQuery.isPending ||
                enableMutation.isPending ||
                rangeInvalid ||
                checked.size === 0
              }
            >
              {enableMutation.isPending && (
                <Loader2 className='animate-spin' aria-hidden='true' />
              )}
              {t('Enable auto sync for {{count}} channels', {
                count: checked.size,
              })}
            </Button>
          </>
        )
      }
    >
      {body}
    </Dialog>
  )
}

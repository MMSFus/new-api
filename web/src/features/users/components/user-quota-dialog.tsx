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
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'

import { adjustUserQuota } from '../api'
import { BALANCE_BUCKET_OPTIONS } from '../lib'
import type {
  BalanceBucketKey,
  QuotaAdjustMode,
  UserBalanceBuckets,
} from '../types'

type BucketTarget = 'total' | BalanceBucketKey

const DEFAULT_TARGET: BucketTarget = 'gift'

interface UserQuotaDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  currentQuota: number
  /** Current balance buckets; enables per-bucket adjustment when present */
  buckets?: UserBalanceBuckets | null
  onSuccess: () => void
}

export function UserQuotaDialog(props: UserQuotaDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<QuotaAdjustMode>('add')
  const [target, setTarget] = useState<BucketTarget>(DEFAULT_TARGET)
  const [amount, setAmount] = useState('')
  const [loading, setLoading] = useState(false)

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  // 增加额度必须指定余额类型（默认赠送余额）；扣减与覆盖可选按默认顺序作用于总额。
  const targets: BucketTarget[] =
    mode === 'add'
      ? BALANCE_BUCKET_OPTIONS.map((o) => o.key)
      : ['total', ...BALANCE_BUCKET_OPTIONS.map((o) => o.key)]

  const amountValue = Number.parseFloat(amount) || 0
  const quotaValue = parseQuotaFromDollars(Math.abs(amountValue))

  const getPreviewText = () => {
    const current =
      target === 'total' || !props.buckets
        ? props.currentQuota
        : props.buckets[target]
    const val = quotaValue
    switch (mode) {
      case 'add':
        return `${t('Current quota')}: ${formatQuota(current)}  +${formatQuota(val)} = ${formatQuota(current + val)}`
      case 'subtract':
        return `${t('Current quota')}: ${formatQuota(current)}  -${formatQuota(val)} = ${formatQuota(current - val)}`
      case 'override': {
        const overrideQuota = parseQuotaFromDollars(amountValue)
        return `${t('Current quota')}: ${formatQuota(current)} → ${formatQuota(overrideQuota)}`
      }
      default:
        return ''
    }
  }

  const handleConfirm = async () => {
    if (!amount && mode !== 'override') return
    if (quotaValue <= 0 && mode !== 'override') return

    setLoading(true)
    try {
      const value =
        mode === 'override' ? parseQuotaFromDollars(amountValue) : quotaValue
      const result = await adjustUserQuota({
        id: props.userId,
        action: 'add_quota',
        mode,
        value: mode === 'override' ? value : Math.abs(value),
        bucket: target === 'total' ? undefined : target,
      })
      if (result.success) {
        toast.success(t('Quota adjusted successfully'))
        setAmount('')
        setMode('add')
        setTarget(DEFAULT_TARGET)
        props.onOpenChange(false)
        props.onSuccess()
      } else {
        handleServerError(result, t('Failed to adjust quota'))
      }
    } catch (e: unknown) {
      handleServerError(e, t('Failed to adjust quota'))
    } finally {
      setLoading(false)
    }
  }

  const handleCancel = () => {
    setAmount('')
    setMode('add')
    setTarget(DEFAULT_TARGET)
    props.onOpenChange(false)
  }

  const placeholder = tokensOnly
    ? t('Enter amount in tokens')
    : t('Enter amount in {{currency}}', { currency: currencyLabel })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Adjust Quota')}
      description={t('Select an operation mode and enter the amount')}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={handleCancel}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={loading}>
            {loading ? t('Processing...') : t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        {props.buckets && (
          <div className='space-y-2'>
            <Label>{t('Balance Breakdown')}</Label>
            <div className='grid grid-cols-2 gap-x-4 gap-y-1 rounded-md border px-3 py-2 text-sm'>
              {BALANCE_BUCKET_OPTIONS.map((option) => (
                <div key={option.key} className='flex justify-between gap-2'>
                  <span className='text-muted-foreground'>
                    {t(option.labelKey)}
                  </span>
                  <span className='font-mono tabular-nums'>
                    {formatQuota(props.buckets?.[option.key] ?? 0)}
                  </span>
                </div>
              ))}
              {props.buckets.debt < 0 && (
                <div className='flex justify-between gap-2'>
                  <span className='text-muted-foreground'>
                    {t('Outstanding Debt')}
                  </span>
                  <span className='text-destructive font-mono tabular-nums'>
                    {formatQuota(props.buckets.debt)}
                  </span>
                </div>
              )}
            </div>
          </div>
        )}

        <div className='space-y-2'>
          <Label>{t('Balance type')}</Label>
          <div className='flex flex-wrap gap-1'>
            {targets.map((key) => (
              <Button
                key={key}
                type='button'
                variant='outline'
                size='sm'
                className={cn(
                  target === key &&
                    'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
                )}
                onClick={() => {
                  setTarget(key)
                  setAmount('')
                }}
              >
                {key === 'total'
                  ? t(mode === 'subtract' ? 'Default order' : 'Total Balance')
                  : t(
                      BALANCE_BUCKET_OPTIONS.find((o) => o.key === key)
                        ?.labelKey ?? key
                    )}
              </Button>
            ))}
          </div>
          {target === 'total' && (
            <div className='text-muted-foreground text-xs'>
              {mode === 'subtract'
                ? t(
                    'Deducts invite reward, referral cashback, gift, then top-up. Any shortfall becomes debt.'
                  )
                : t(
                    'Overriding the total credits the difference to the gift balance, or deducts it in the default order.'
                  )}
            </div>
          )}
        </div>

        <div className='text-muted-foreground text-sm'>{getPreviewText()}</div>

        <div className='space-y-2'>
          <Label>{t('Mode')}</Label>
          <div className='flex gap-1'>
            {(['add', 'subtract', 'override'] as const).map((m) => (
              <Button
                key={m}
                type='button'
                variant='outline'
                size='sm'
                className={cn(
                  mode === m &&
                    'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
                )}
                onClick={() => {
                  setMode(m)
                  if (m === 'add' && target === 'total') {
                    setTarget(DEFAULT_TARGET)
                  }
                  setAmount('')
                }}
              >
                {m === 'add' && t('Add')}
                {!(m === 'add') && m === 'subtract' && t('Subtract')}
                {!(m === 'add') && !(m === 'subtract') && t('Override')}
              </Button>
            ))}
          </div>
        </div>

        <div className='space-y-2'>
          <Label>
            {t('Amount')} ({currencyLabel})
          </Label>
          <Input
            type='number'
            step={tokensOnly ? 1 : 0.000001}
            min={mode === 'override' && target === 'total' ? undefined : 0}
            placeholder={placeholder}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleConfirm()
            }}
          />
        </div>
      </div>
    </Dialog>
  )
}

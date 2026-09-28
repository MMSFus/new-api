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
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'

import type { BalanceBuckets, UserWalletData } from '../types'

interface BalanceBucketsCardProps {
  user: UserWalletData | null
  loading?: boolean
}

const BUCKET_ITEMS: {
  key: keyof BalanceBuckets
  labelKey: string
  descriptionKey: string
}[] = [
  {
    key: 'topup',
    labelKey: 'Top-up Balance',
    descriptionKey: 'From online payments and redemption codes',
  },
  {
    key: 'aff_rebate',
    labelKey: 'Referral Cashback',
    descriptionKey: 'Transferred from referral rewards',
  },
  {
    key: 'invite_bonus',
    labelKey: 'Invite Reward',
    descriptionKey: 'Granted for signing up with an invite code',
  },
  {
    key: 'gift',
    labelKey: 'Gift Balance',
    descriptionKey: 'New-user gift, check-in and admin grants',
  },
]

export function BalanceBucketsCard(props: BalanceBucketsCardProps) {
  const { t } = useTranslation()

  if (props.loading) {
    return <Skeleton className='h-20 w-full rounded-lg' />
  }

  const buckets = props.user?.balance_buckets
  if (!buckets) return null

  const items = BUCKET_ITEMS.map((item) => ({
    key: item.key,
    label: t(item.labelKey),
    description: t(item.descriptionKey),
    value: buckets[item.key],
  }))
  if (buckets.debt < 0) {
    items.push({
      key: 'debt',
      label: t('Outstanding Debt'),
      description: t('Repaid automatically by the next credit'),
      value: buckets.debt,
    })
  }

  return (
    <div className='rounded-lg border'>
      <div className='flex items-baseline justify-between gap-2 border-b px-3 py-2 sm:px-5'>
        <div className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
          {t('Balance Breakdown')}
        </div>
        <div className='text-muted-foreground text-xs'>
          {t('Total Balance')}:{' '}
          <span className='text-foreground font-mono font-semibold tabular-nums'>
            {formatQuota(props.user?.quota ?? 0)}
          </span>
        </div>
      </div>
      <div className='grid grid-cols-2 divide-y sm:grid-cols-4 sm:divide-x sm:divide-y-0'>
        {items.map((item) => (
          <div key={item.key} className='min-w-0 px-3 py-2.5 sm:px-5 sm:py-3'>
            <div className='text-muted-foreground truncate text-xs'>
              {item.label}
            </div>
            <div
              className={
                item.value < 0
                  ? 'text-destructive mt-1 font-mono text-sm font-semibold tabular-nums sm:text-base'
                  : 'text-foreground mt-1 font-mono text-sm font-semibold tabular-nums sm:text-base'
              }
            >
              {formatQuota(item.value)}
            </div>
            <div className='text-muted-foreground/60 mt-0.5 hidden text-xs md:block'>
              {item.description}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

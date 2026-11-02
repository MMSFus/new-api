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

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'

import { useSyncExchangeRate } from '../hooks/use-update-option'

export type ExchangeRateSyncStatusValue = {
  lastSyncedAt: number
  lastError: string
  lastSource: string
}

type ExchangeRateSyncStatusProps = {
  status: ExchangeRateSyncStatusValue
  // Syncing refreshes the form from the server, which would drop unsaved edits.
  disabled: boolean
}

export function ExchangeRateSyncStatus(props: ExchangeRateSyncStatusProps) {
  const { t } = useTranslation()
  const syncExchangeRate = useSyncExchangeRate()
  const hasSynced = props.status.lastSyncedAt > 0

  return (
    <div className='flex flex-wrap items-center gap-x-4 gap-y-2 text-sm'>
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={props.disabled || syncExchangeRate.isPending}
        onClick={() => syncExchangeRate.mutate()}
      >
        {t('Sync Now')}
      </Button>
      <span className='text-muted-foreground'>
        {t('Last synced')}: {formatTimestampToDate(props.status.lastSyncedAt)}
      </span>
      {hasSynced && (
        <StatusBadge
          copyable={false}
          variant={props.status.lastError ? 'danger' : 'success'}
          label={
            props.status.lastError ? t(props.status.lastError) : t('Synced')
          }
        />
      )}
      {props.status.lastSource && (
        <span className='text-muted-foreground'>
          {t('Source')}: {props.status.lastSource}
        </span>
      )}
    </div>
  )
}

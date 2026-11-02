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
import { History } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatCompactNumber } from '@/lib/format'

import type { ChannelCostFilters } from './api'
import { ChannelCostBackfillPanel } from './backfill-panel'
import { useCostBackfillStatus } from './use-cost-backfill'

/**
 * Header entry of the low-frequency historical backfill. It stays mounted, so
 * the task keeps polling and refreshes the report while the dialog is closed.
 */
export function ChannelCostBackfillEntry(props: {
  filters: ChannelCostFilters
  missing?: number
  enabled: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [open, setOpen] = useState(false)
  const backfill = useCostBackfillStatus({ trackCompletion: true })
  if (!backfill.isRoot) return null

  const missing = props.missing ?? 0
  let badge = null
  if (backfill.active) {
    badge = <Badge variant='secondary'>{t('Running')}</Badge>
  } else if (missing > 0) {
    badge = (
      <Badge variant='warning' title={t('Uncosted requests')}>
        {formatCompactNumber(missing, locale)}
      </Badge>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      title={t('Historical cost backfill')}
      description={t(
        'Backfill historical costs using current channel ratios. Eligible records will then count toward cost and profit.'
      )}
      contentClassName='sm:max-w-2xl'
      trigger={
        <Button type='button' variant='outline' size='sm'>
          <History aria-hidden='true' />
          {t('Historical cost backfill')}
          {badge}
        </Button>
      }
    >
      <ChannelCostBackfillPanel
        filters={props.filters}
        missing={props.missing}
        enabled={props.enabled}
        trackCompletion={false}
      />
      {missing === 0 && !backfill.task && (
        <p className='text-muted-foreground text-sm'>
          {t('Every request in the current range has a cost.')}
        </p>
      )}
    </Dialog>
  )
}

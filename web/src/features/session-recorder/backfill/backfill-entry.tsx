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

import { BackfillPanel } from './backfill-panel'
import { useBackfill } from './use-backfill'

/**
 * Header entry of the root-only historical index backfill. It stays mounted
 * so a running task keeps polling while the dialog is closed.
 */
export function BackfillEntry() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const backfill = useBackfill(true)

  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      title={t('Historical index backfill')}
      description={t(
        'Index recordings uploaded before the index existed. Until then they are missing from revenue and conversation review.'
      )}
      contentClassName='sm:max-w-2xl'
      trigger={
        <Button type='button' variant='outline' size='sm'>
          <History aria-hidden='true' />
          {t('Historical index backfill')}
          {backfill.running && (
            <Badge variant='secondary'>{t('Running')}</Badge>
          )}
        </Button>
      }
    >
      <BackfillPanel backfill={backfill} />
    </Dialog>
  )
}

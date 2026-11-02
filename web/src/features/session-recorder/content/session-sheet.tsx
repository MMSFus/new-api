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
import { ArrowLeft } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { SessionRecord, SessionSummary } from '../api'
import { formatUnixTime } from '../lib'
import { useSessionRecords } from '../use-session-recorder-data'
import { RecordContentView } from './record-content'
import { RecordTimeline } from './record-timeline'

/** Session drill sheet: its record timeline, then one record's content. */
export function SessionSheet(props: {
  session: SessionSummary | null
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Sheet open={props.session !== null} onOpenChange={props.onOpenChange}>
      <SheetContent
        className={sideDrawerContentClassName('sm:max-w-3xl xl:max-w-5xl')}
      >
        {props.session && (
          <SessionContent
            key={props.session.session_id}
            session={props.session}
          />
        )}
      </SheetContent>
    </Sheet>
  )
}

function SessionContent(props: { session: SessionSummary }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [selected, setSelected] = useState<SessionRecord | null>(null)
  const query = useSessionRecords(props.session.session_id)

  if (selected) {
    return (
      <>
        <SheetHeader className={sideDrawerHeaderClassName('pr-12')}>
          <SheetTitle>{t('Record content')}</SheetTitle>
          <SheetDescription className='font-mono break-all tabular-nums'>
            {selected.model_name} · {formatUnixTime(selected.created_at)} ·{' '}
            {selected.record_id}
          </SheetDescription>
        </SheetHeader>
        <div className={sideDrawerFormClassName('gap-4')}>
          <div>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => setSelected(null)}
            >
              <ArrowLeft aria-hidden='true' />
              {t('Back')}
            </Button>
          </div>
          <RecordContentView key={selected.record_id} record={selected} />
        </div>
      </>
    )
  }

  let body = <LoadingState />
  if (query.isError) {
    body = (
      <ErrorState
        title={t('Failed to load session records')}
        description={query.error.message}
        onRetry={() => void query.refetch()}
      />
    )
  } else if (query.data && query.data.items.length === 0) {
    body = <EmptyState title={t('No records in this session')} bordered />
  } else if (query.data) {
    const list = query.data
    body = (
      <>
        {list.total > list.limit && (
          <Alert>
            <AlertDescription>
              {t(
                'This session has {{total}} records; only the first {{limit}} are shown.',
                {
                  total: formatNumber(list.total, locale),
                  limit: formatNumber(list.limit, locale),
                }
              )}
            </AlertDescription>
          </Alert>
        )}
        <RecordTimeline records={list.items} onOpen={setSelected} />
      </>
    )
  }

  return (
    <>
      <SheetHeader className={sideDrawerHeaderClassName('pr-12')}>
        <SheetTitle>{t('Session')}</SheetTitle>
        <SheetDescription className='font-mono break-all tabular-nums'>
          {props.session.session_id} · {formatUnixTime(props.session.first_at)}
          {' ~ '}
          {formatUnixTime(props.session.last_at)}
        </SheetDescription>
      </SheetHeader>
      <div className={sideDrawerFormClassName('gap-4')}>{body}</div>
    </>
  )
}

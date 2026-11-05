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
import { AlertTriangle } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { RecordContent, SessionRecord } from '../../api'
import { PartList } from './part-view'
import type { Conversation, Role, Turn } from './types'
import { useRecordConversation } from './use-conversation'

/** Request turns shown before "Show earlier" is clicked. */
const RECENT_TURNS = 6

function TurnView(props: { turn: Turn; markdown: boolean; filename: string }) {
  const { t } = useTranslation()
  const user = props.turn.role === 'user' || props.turn.role === 'tool'
  const roleLabels: Record<Role, string> = {
    system: t('System'),
    user: t('User'),
    assistant: t('Assistant'),
    tool: t('Tool'),
  }
  const role = roleLabels[props.turn.role]
  return (
    <li
      aria-label={role}
      className={cn('flex min-w-0 flex-col gap-1', user && 'items-end')}
    >
      <span className='text-muted-foreground text-xs font-medium'>
        {role}
        {props.turn.label && ` ${props.turn.label}`}
      </span>
      <div
        className={cn(
          'min-w-0 rounded-lg',
          user && 'bg-secondary max-w-[90%] px-3 py-2',
          props.turn.role === 'system' && 'bg-muted/50 w-full border px-3 py-2'
        )}
      >
        <PartList
          parts={props.turn.parts}
          markdown={props.markdown}
          filename={props.filename}
        />
      </div>
    </li>
  )
}

function TurnList(props: {
  turns: Turn[]
  markdown: boolean
  filename: string
  collapse: boolean
}) {
  const { t } = useTranslation()
  const [showAll, setShowAll] = useState(false)
  const hidden =
    props.collapse && !showAll
      ? Math.max(0, props.turns.length - RECENT_TURNS)
      : 0
  // Turns have no ids; their recorded position is their identity.
  const visible = props.turns
    .slice(hidden)
    .map((turn, offset) => ({ turn, position: hidden + offset }))
  return (
    <ol className='flex flex-col gap-4'>
      {hidden > 0 && (
        <li className='flex justify-center'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => setShowAll(true)}
          >
            {t('Show {{count}} earlier messages', { count: hidden })}
          </Button>
        </li>
      )}
      {visible.map((item) => (
        <TurnView
          key={item.position}
          turn={item.turn}
          markdown={props.markdown}
          filename={`${props.filename}-${item.position + 1}`}
        />
      ))}
    </ol>
  )
}

function ConversationBody(props: {
  conversation: Conversation
  fullBodyFailed: boolean
  recordId: string
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [markdown, setMarkdown] = useState(true)
  const conversation = props.conversation
  const params = Object.entries(conversation.params)
  const usage = Object.entries(conversation.usage ?? {})

  return (
    <div className='space-y-5'>
      {conversation.error && (
        <Alert variant='destructive'>
          <AlertTriangle aria-hidden='true' />
          <AlertDescription className='break-words whitespace-pre-wrap'>
            {conversation.error}
          </AlertDescription>
        </Alert>
      )}
      {conversation.partial && (
        <Alert>
          <AlertTriangle aria-hidden='true' />
          <AlertDescription>
            {props.fullBodyFailed
              ? t(
                  'The full body could not be loaded, so only the first 2 MiB is shown. See the Raw tab for the stored data.'
                )
              : t(
                  'The recorded body is incomplete, so this view may be missing content. See the Raw tab for the stored data.'
                )}
          </AlertDescription>
        </Alert>
      )}
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <ul
          aria-label={t('Request parameters')}
          className='flex flex-wrap gap-1.5'
        >
          {params.map(([key, value]) => (
            <li key={key}>
              <Badge variant='outline' className='font-mono'>
                {key}: {value}
              </Badge>
            </li>
          ))}
        </ul>
        <label className='text-muted-foreground flex items-center gap-2 text-xs'>
          <Switch checked={markdown} onCheckedChange={setMarkdown} />
          {t('Render Markdown')}
        </label>
      </div>
      <section aria-label={t('Request')} className='space-y-3'>
        <h3 className='text-sm font-medium'>{t('Request')}</h3>
        {conversation.request.length === 0 && (
          <p className='text-muted-foreground text-sm'>{t('No messages')}</p>
        )}
        <TurnList
          turns={conversation.request}
          markdown={markdown}
          filename={`${props.recordId}-request`}
          collapse
        />
      </section>
      <section aria-label={t('Response')} className='space-y-3'>
        <h3 className='text-sm font-medium'>{t('Response')}</h3>
        {conversation.response.length === 0 && (
          <p className='text-muted-foreground text-sm'>{t('No messages')}</p>
        )}
        <TurnList
          turns={conversation.response}
          markdown={markdown}
          filename={`${props.recordId}-response`}
          collapse={false}
        />
        {(usage.length > 0 || conversation.finishReason) && (
          <ul
            aria-label={t('Usage')}
            className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 font-mono text-xs'
          >
            {conversation.finishReason && (
              <li>finish_reason: {conversation.finishReason}</li>
            )}
            {usage.map(([key, value]) => (
              <li key={key}>
                {key}: {formatNumber(value, locale)}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}

/** The recorded exchange as chat turns and images. */
export function ConversationView(props: {
  record: SessionRecord
  content: RecordContent
}) {
  const { t } = useTranslation()
  const query = useRecordConversation(props.record, props.content)
  if (query.isPending) return <LoadingState />
  if (query.isError) {
    return (
      <ErrorState
        title={t('Failed to parse the conversation')}
        description={query.error.message}
        onRetry={() => void query.refetch()}
      />
    )
  }
  if (!query.data.conversation) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('This record format has no conversation view. Use the Raw tab.')}
      </p>
    )
  }
  return (
    <ConversationBody
      conversation={query.data.conversation}
      fullBodyFailed={query.data.fullBodyFailed}
      recordId={props.record.record_id}
    />
  )
}

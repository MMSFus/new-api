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
import { Archive } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { SessionRecord } from '../api'
import { useRecordContent } from '../use-session-recorder-data'
import { ConversationView } from './conversation/conversation-view'
import { detectFormat } from './conversation/parse'
import { RecordBodyView } from './record-body'

/** Request, response and raw record of one recorded request. */
export function RecordContentView(props: { record: SessionRecord }) {
  const { t } = useTranslation()
  const recordId = props.record.record_id
  const query = useRecordContent(recordId)
  const archived = props.record.archived_at > 0

  if (query.isPending) {
    return (
      <LoadingState
        message={
          archived
            ? t('Reading from the archive. The first read can take a while.')
            : undefined
        }
      />
    )
  }
  if (query.isError) {
    return (
      <ErrorState
        title={t('Failed to load record content')}
        description={query.error.message}
        onRetry={() => void query.refetch()}
      />
    )
  }

  const content = query.data
  const requestText =
    content.request?.encoding === 'utf8' ? content.request.text : null
  const conversational = detectFormat(props.record.path, requestText) !== null
  return (
    <div className='space-y-5'>
      {content.source === 'archive' && (
        <Alert>
          <Archive aria-hidden='true' />
          <AlertDescription>
            {t(
              'Read from the archive because the local copy was cleaned up. The first read is slower; repeat reads are cached for a while.'
            )}
          </AlertDescription>
        </Alert>
      )}
      <Tabs defaultValue={conversational ? 'conversation' : 'raw'}>
        <TabsList aria-label={t('Content view')}>
          {conversational && (
            <TabsTrigger value='conversation'>{t('Conversation')}</TabsTrigger>
          )}
          <TabsTrigger value='raw'>{t('Raw')}</TabsTrigger>
        </TabsList>
        {conversational && (
          <TabsContent value='conversation' className='pt-3'>
            <ConversationView record={props.record} content={content} />
          </TabsContent>
        )}
        <TabsContent value='raw' className='space-y-5 pt-3'>
          <RecordBodyView
            title={t('Request')}
            body={content.request}
            filename={`${recordId}-request`}
          />
          <RecordBodyView
            title={t('Response')}
            body={content.response}
            filename={`${recordId}-response`}
          />
          <section aria-label={t('Record metadata')} className='space-y-2'>
            <h3 className='text-sm font-medium'>{t('Record metadata')}</h3>
            <CodeBlock
              code={JSON.stringify(content.record, null, 2)}
              language='json'
              title={t('Record metadata')}
              filename={`${recordId}-record.json`}
              showToolbar
              defaultCollapsed
              className='my-0'
            >
              <CodeBlockCopyButton />
            </CodeBlock>
          </section>
        </TabsContent>
      </Tabs>
    </div>
  )
}

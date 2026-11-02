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

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toIntlLocale } from '@/i18n/languages'

import type { RecordBody } from '../api'
import { formatDecimalBytes } from '../lib'
import { bodyView } from './body-view'

export function RecordBodyView(props: {
  title: string
  body: RecordBody | null
  /** Download name without extension. */
  filename: string
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const body = props.body

  if (!body) {
    return (
      <section aria-label={props.title} className='space-y-2'>
        <h3 className='text-sm font-medium'>{props.title}</h3>
        <p className='text-muted-foreground text-sm'>
          {t('No body was recorded.')}
        </p>
      </section>
    )
  }

  const view = bodyView(body)
  return (
    <section aria-label={props.title} className='space-y-2'>
      <div className='flex flex-wrap items-baseline justify-between gap-2'>
        <h3 className='text-sm font-medium'>{props.title}</h3>
        <span className='text-muted-foreground font-mono text-xs'>
          {body.content_type || '-'} · {formatDecimalBytes(body.size, locale)}
        </span>
      </div>
      {body.truncated && (
        <Alert>
          <AlertDescription>
            {t(
              'Truncated: only the first 2 MiB of {{size}} is shown. Copy and download contain the shown part only.',
              { size: formatDecimalBytes(body.size, locale) }
            )}
          </AlertDescription>
        </Alert>
      )}
      {body.encoding === 'base64' && (
        <p className='text-muted-foreground text-xs'>
          {t('Binary body, shown base64-encoded.')}
        </p>
      )}
      <CodeBlock
        code={view.code}
        language={view.language}
        title={props.title}
        filename={`${props.filename}.${view.extension}`}
        showToolbar
        collapsedLines={20}
        maxExpandedLines={60}
        className='my-0'
      >
        <CodeBlockCopyButton />
      </CodeBlock>
    </section>
  )
}

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
import { BrainIcon, ChevronDownIcon, FileIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import {
  Reasoning,
  ReasoningContent,
  ReasoningTrigger,
} from '@/components/ai-elements/reasoning'
import { Response } from '@/components/ai-elements/response'
import { Tool, ToolContent, ToolHeader } from '@/components/ai-elements/tool'
import { Button } from '@/components/ui/button'

import { ImageGallery } from './image-gallery'
import type { Part } from './types'

const LONG_TEXT = 4000

/**
 * Markdown images that `splitMarkdownImages` did not take out (reference
 * style, HTML img, odd URLs) would be fetched by the markdown renderer as
 * soon as the record opens; such text is shown verbatim instead.
 */
const REMAINING_IMAGE = /!\[[^\]]*\]\s*[([]|<img\b/i

/** Text of a message: markdown when it is safe to render, else verbatim. */
export function TextBlock(props: { text: string; markdown: boolean }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const long = props.text.length > LONG_TEXT
  const text =
    long && !expanded ? `${props.text.slice(0, LONG_TEXT)}…` : props.text
  const markdown = props.markdown && !REMAINING_IMAGE.test(props.text)

  return (
    <div className='min-w-0 space-y-1'>
      {markdown ? (
        <Response className='text-sm break-words'>{text}</Response>
      ) : (
        <p className='text-sm break-words whitespace-pre-wrap'>{text}</p>
      )}
      {long && (
        <Button
          type='button'
          variant='link'
          size='sm'
          className='h-auto px-0'
          onClick={() => setExpanded(!expanded)}
        >
          {expanded
            ? t('Show less')
            : t('Show all ({{count}} characters)', {
                count: props.text.length,
              })}
        </Button>
      )}
    </div>
  )
}

function ToolBlock(props: {
  part: Extract<Part, { type: 'tool_call' | 'tool_result' }>
}) {
  const { t } = useTranslation()
  const part = props.part
  const call = part.type === 'tool_call'
  const code = call ? part.input : part.output
  let state: 'output-available' | 'output-error' = 'output-available'
  if (!call && part.isError) state = 'output-error'
  const name = part.name || (call ? t('Tool call') : t('Tool result'))

  return (
    <Tool className='mb-0'>
      <ToolHeader
        type='tool-call'
        state={state}
        title={call ? name : t('Result: {{name}}', { name })}
      />
      <ToolContent>
        <div className='space-y-1 px-3 pb-3'>
          {part.id && (
            <p className='text-muted-foreground font-mono text-xs break-all'>
              {part.id}
            </p>
          )}
          <CodeBlock
            code={code || '-'}
            language='json'
            collapsedLines={12}
            maxExpandedLines={60}
            className='my-0'
          >
            <CodeBlockCopyButton />
          </CodeBlock>
        </div>
      </ToolContent>
    </Tool>
  )
}

/** One part of a turn. Image parts are rendered by `PartList` as a gallery. */
function PartView(props: { part: Part; markdown: boolean }) {
  const { t } = useTranslation()
  const part = props.part
  if (part.type === 'text') {
    return <TextBlock text={part.text} markdown={props.markdown} />
  }
  if (part.type === 'tool_call' || part.type === 'tool_result') {
    return <ToolBlock part={part} />
  }
  if (part.type === 'file') {
    return (
      <p className='text-muted-foreground flex items-center gap-1.5 text-xs'>
        <FileIcon aria-hidden='true' className='size-3.5' />
        <span className='font-mono break-all'>
          {[part.name, part.mediaType].filter(Boolean).join(' · ') || t('File')}
        </span>
      </p>
    )
  }
  if (part.type !== 'reasoning') return null
  if (part.redacted || !part.text) {
    return (
      <p className='text-muted-foreground text-xs italic'>
        {t('Reasoning is encrypted and cannot be shown.')}
      </p>
    )
  }
  return (
    <Reasoning defaultOpen={false} className='mb-0'>
      <ReasoningTrigger>
        <BrainIcon aria-hidden='true' className='size-3.5' />
        <span>{t('Reasoning')}</span>
        <ChevronDownIcon aria-hidden='true' className='size-3.5' />
      </ReasoningTrigger>
      <ReasoningContent>
        {REMAINING_IMAGE.test(part.text)
          ? `\`\`\`text\n${part.text}\n\`\`\``
          : part.text}
      </ReasoningContent>
    </Reasoning>
  )
}

/** Parts in order; consecutive images share one gallery. */
export function PartList(props: {
  parts: Part[]
  markdown: boolean
  filename: string
}) {
  // Parts have no ids; their position in the message is their identity.
  const blocks: (
    | { type: 'part'; position: number; part: Part }
    | {
        type: 'gallery'
        position: number
        images: Extract<Part, { type: 'image' }>[]
      }
  )[] = []
  for (const [position, part] of props.parts.entries()) {
    const tail = blocks.at(-1)
    if (part.type !== 'image') blocks.push({ type: 'part', position, part })
    else if (tail?.type === 'gallery') tail.images.push(part)
    else blocks.push({ type: 'gallery', position, images: [part] })
  }
  return (
    <div className='flex min-w-0 flex-col gap-2'>
      {blocks.map((block) =>
        block.type === 'gallery' ? (
          <ImageGallery
            key={block.position}
            images={block.images}
            filename={`${props.filename}-${block.position + 1}`}
          />
        ) : (
          <PartView
            key={block.position}
            part={block.part}
            markdown={props.markdown}
          />
        )
      )}
    </div>
  )
}

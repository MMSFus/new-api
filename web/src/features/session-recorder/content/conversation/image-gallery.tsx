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
import { Download, ExternalLink, ImageOff } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { ImageSource } from './types'

export interface GalleryImage {
  source: ImageSource
  caption?: string
}

function imageSrc(source: ImageSource): string {
  if (source.kind === 'url') return source.url
  return `data:${source.mediaType};base64,${source.base64}`
}

function hostOf(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}

/**
 * Recorded images in a grid; a click opens one large with download.
 *
 * Not `ai-elements/image`: that component only takes inline base64 and has
 * no remote-URL, deferred-load or preview behaviour. Remote images are not
 * fetched until the reviewer asks, so opening a record does not tell the
 * image host (often chosen by the end user) that it is being reviewed; the
 * request then carries no referrer.
 */
export function ImageGallery(props: {
  images: GalleryImage[]
  filename: string
}) {
  const { t } = useTranslation()
  const [allowRemote, setAllowRemote] = useState(false)
  const [open, setOpen] = useState<number | null>(null)
  const current = open === null ? null : props.images[open]
  const remoteCount = props.images.filter(
    (image) => image.source.kind === 'url'
  ).length
  const loadRemote = (
    <Button
      type='button'
      variant='outline'
      size='sm'
      onClick={() => setAllowRemote(true)}
    >
      {t('Load external images')}
    </Button>
  )

  return (
    <div className='space-y-2'>
      {remoteCount > 0 && !allowRemote && (
        <div className='text-muted-foreground flex flex-wrap items-center gap-2 text-xs'>
          <span>
            {t('{{count}} external images are not loaded.', {
              count: remoteCount,
            })}
          </span>
          {loadRemote}
        </div>
      )}
      <ul
        aria-label={t('Images ({{count}})', { count: props.images.length })}
        className={cn(
          'grid gap-2',
          props.images.length > 1
            ? 'grid-cols-2 sm:grid-cols-3'
            : 'grid-cols-1 sm:max-w-md'
        )}
      >
        {props.images.map((image, index) => {
          const name = t('Image {{index}}', { index: index + 1 })
          return (
            // Images are positional: the response order is their identity.
            // eslint-disable-next-line react/no-array-index-key
            <li key={index} className='space-y-1'>
              <button
                type='button'
                aria-label={t('Open {{name}}', { name })}
                className='bg-muted/40 focus-visible:ring-ring flex aspect-square w-full items-center justify-center overflow-hidden rounded-md border focus-visible:ring-2 focus-visible:outline-none'
                onClick={() => setOpen(index)}
              >
                {image.source.kind === 'url' && !allowRemote ? (
                  <span className='text-muted-foreground flex flex-col items-center gap-1 p-2 text-xs break-all'>
                    <ImageOff aria-hidden='true' className='size-5' />
                    {hostOf(image.source.url)}
                  </span>
                ) : (
                  <img
                    alt={image.caption || name}
                    src={imageSrc(image.source)}
                    loading='lazy'
                    referrerPolicy='no-referrer'
                    className='size-full object-contain'
                  />
                )}
              </button>
              {image.caption && (
                <p
                  className='text-muted-foreground line-clamp-2 text-xs'
                  title={image.caption}
                >
                  {image.caption}
                </p>
              )}
            </li>
          )
        })}
      </ul>
      <Dialog
        open={current !== null}
        onOpenChange={(next) => !next && setOpen(null)}
        title={t('Image {{index}}', { index: (open ?? 0) + 1 })}
        contentClassName='sm:max-w-4xl'
        footer={
          current && (
            <ImageActions
              image={current}
              filename={`${props.filename}-${(open ?? 0) + 1}`}
            />
          )
        }
      >
        {current && (
          <div className='space-y-3'>
            {current.source.kind === 'url' && !allowRemote ? (
              <div className='flex flex-col items-center gap-2 py-6 text-sm'>
                <p className='text-muted-foreground break-all'>
                  {current.source.url}
                </p>
                {loadRemote}
              </div>
            ) : (
              <img
                alt={
                  current.caption ||
                  t('Image {{index}}', { index: (open ?? 0) + 1 })
                }
                src={imageSrc(current.source)}
                referrerPolicy='no-referrer'
                className='mx-auto max-h-[70vh] w-auto object-contain'
              />
            )}
            {current.caption && (
              <p className='text-muted-foreground text-sm whitespace-pre-wrap'>
                {current.caption}
              </p>
            )}
          </div>
        )}
      </Dialog>
    </div>
  )
}

function ImageActions(props: { image: GalleryImage; filename: string }) {
  const { t } = useTranslation()
  const source = props.image.source
  return (
    <div className='flex flex-wrap justify-end gap-2'>
      {props.image.caption && (
        <CopyButton
          value={props.image.caption}
          variant='outline'
          size='sm'
          tooltip={t('Copy prompt')}
        >
          {t('Copy prompt')}
        </CopyButton>
      )}
      {source.kind === 'url' ? (
        <Button
          role='link'
          variant='outline'
          size='sm'
          render={
            <a href={source.url} target='_blank' rel='noopener noreferrer' />
          }
        >
          <ExternalLink aria-hidden='true' />
          {t('Open original')}
        </Button>
      ) : (
        <Button
          role='link'
          variant='outline'
          size='sm'
          render={
            <a
              href={imageSrc(source)}
              download={`${props.filename}.${source.mediaType.split('/')[1] || 'png'}`}
            />
          }
        >
          <Download aria-hidden='true' />
          {t('Download')}
        </Button>
      )}
    </div>
  )
}

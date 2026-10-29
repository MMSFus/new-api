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
import { useMutation } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import {
  exchangeAntigravityOAuth,
  importAntigravityCredentials,
  startAntigravityOAuth,
  type AntigravityCredentialResult,
} from '../api'

export function AntigravityCredentials(props: {
  disabled: boolean
  proxy: string
  value: string
  onImport: (result: AntigravityCredentialResult) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const active = useRef(true)
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
    }
  }, [])
  const [authorizationUrl, setAuthorizationUrl] = useState('')
  const [callbackUrl, setCallbackUrl] = useState('')
  const [imported, setImported] = useState<number | null>(null)
  const start = useMutation({
    mutationFn: () => startAntigravityOAuth(props.proxy),
    onSuccess: (result) => {
      if (!active.current) return
      setAuthorizationUrl(result.authorization_url)
      setCallbackUrl('')
    },
  })
  const exchange = useMutation({
    mutationFn: () => exchangeAntigravityOAuth(callbackUrl),
    onSuccess: (result) => {
      if (!active.current) return
      props.onImport(result)
      setImported(result.count)
      setAuthorizationUrl('')
      setCallbackUrl('')
    },
  })
  const importedCredentials = useMutation({
    mutationFn: async (files: File[] | null) => {
      if (!files) return importAntigravityCredentials(props.value)
      if (
        files.length > 500 ||
        files.reduce((size, file) => size + file.size, 0) > 8 * 1024 * 1024
      ) {
        throw new Error(t('Antigravity import exceeds size limit'))
      }
      const values: unknown[] = []
      for (const file of files) {
        try {
          values.push(JSON.parse(await file.text()) as unknown)
        } catch {
          throw new Error(t('Invalid Antigravity credential JSON'))
        }
      }
      return importAntigravityCredentials(JSON.stringify(values))
    },
    onSuccess: (result) => {
      if (!active.current) return
      props.onImport(result)
      setImported(result.count)
    },
    onError: (error) =>
      handleServerError(error, t('Invalid Antigravity credential JSON')),
  })
  const busy =
    props.disabled ||
    start.isPending ||
    exchange.isPending ||
    importedCredentials.isPending
  return (
    <div className='space-y-3 rounded-lg border p-3'>
      <p className='text-sm font-medium'>{t('Antigravity credentials')}</p>
      <div className='flex flex-wrap items-center gap-2'>
        <Button
          type='button'
          size='sm'
          disabled={busy}
          onClick={() => start.mutate()}
        >
          {t('Sign in with Google')}
        </Button>
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={busy || !props.value.trim()}
          onClick={() => importedCredentials.mutate(null)}
        >
          {t('Import pasted JSON')}
        </Button>
      </div>
      {authorizationUrl && (
        <div className='space-y-2'>
          <div className='flex items-center gap-2'>
            <a
              href={authorizationUrl}
              target='_blank'
              rel='noopener noreferrer'
              className='text-primary text-sm underline'
            >
              {t('Open Google authorization')}
            </a>
            <CopyButton
              value={authorizationUrl}
              size='sm'
              aria-label={t('Copy authorization link')}
            />
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'After authorization, copy the complete localhost callback URL from the address bar and paste it below. The link expires in 10 minutes.'
            )}
          </p>
          <Label htmlFor={`${id}-callback`}>{t('Complete callback URL')}</Label>
          <Input
            id={`${id}-callback`}
            value={callbackUrl}
            onChange={(event) => setCallbackUrl(event.target.value)}
            disabled={busy}
            autoComplete='off'
          />
          <Button
            type='button'
            size='sm'
            disabled={busy || !callbackUrl.trim()}
            onClick={() => exchange.mutate()}
          >
            {t('Complete authorization')}
          </Button>
        </div>
      )}
      <div className='space-y-2'>
        <Label htmlFor={`${id}-files`}>
          {t('Import sub2api or CPA JSON files')}
        </Label>
        <Input
          id={`${id}-files`}
          type='file'
          accept='.json,application/json'
          multiple
          disabled={busy}
          onChange={(event) => {
            const files = [...(event.target.files ?? [])]
            event.target.value = ''
            if (files.length) importedCredentials.mutate(files)
          }}
        />
      </div>
      {imported !== null && (
        <p role='status' className='text-muted-foreground text-xs'>
          {t('Imported {{count}} accounts. Save the channel to apply.', {
            count: imported,
          })}
        </p>
      )}
      <p className='text-muted-foreground text-xs'>
        {t(
          'Imported credentials are added to this form. Proxy settings and remote API addresses from backups are ignored.'
        )}
      </p>
    </div>
  )
}

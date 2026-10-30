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
import { ClipboardPaste, KeyRound, LoaderCircle, LogIn } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from '@/components/ui/input-group'
import { handleServerError } from '@/lib/handle-server-error'

import {
  exchangeAntigravityOAuth,
  importAntigravityCredentials,
  startAntigravityOAuth,
  type AntigravityCredentialResult,
} from '../api'
import {
  AuthorizationLink,
  CredentialNotice,
  CredentialPanel,
  CredentialStep,
} from './credential-panel'

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
    <CredentialPanel
      icon={<KeyRound />}
      title={t('Antigravity credentials')}
      description={t('Connect directly to Antigravity with Google OAuth')}
    >
      <div className='flex flex-wrap items-center gap-2'>
        <Button
          type='button'
          size='sm'
          disabled={busy}
          onClick={() => start.mutate()}
        >
          {start.isPending ? (
            <LoaderCircle className='animate-spin' aria-hidden='true' />
          ) : (
            <LogIn aria-hidden='true' />
          )}
          {t('Sign in with Google')}
        </Button>
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={busy || !props.value.trim()}
          onClick={() => importedCredentials.mutate(null)}
        >
          <ClipboardPaste aria-hidden='true' />
          {t('Import pasted JSON')}
        </Button>
      </div>
      {authorizationUrl && (
        <CredentialStep>
          <div className='flex flex-wrap items-center gap-2'>
            <AuthorizationLink href={authorizationUrl}>
              {t('Open Google authorization')}
            </AuthorizationLink>
            <CopyButton
              value={authorizationUrl}
              variant='outline'
              className='size-7'
              tooltip={t('Copy authorization link')}
              aria-label={t('Copy authorization link')}
            />
          </div>
          <Field>
            <FieldLabel htmlFor={`${id}-callback`}>
              {t('Complete callback URL')}
            </FieldLabel>
            <FieldDescription className='text-xs'>
              {t(
                'After authorization, copy the complete localhost callback URL from the address bar and paste it below. The link expires in 10 minutes.'
              )}
            </FieldDescription>
            <InputGroup>
              <InputGroupInput
                id={`${id}-callback`}
                value={callbackUrl}
                onChange={(event) => setCallbackUrl(event.target.value)}
                disabled={busy}
                autoComplete='off'
                className='font-mono text-xs'
              />
              <InputGroupAddon align='inline-end'>
                <InputGroupButton
                  variant='default'
                  disabled={busy || !callbackUrl.trim()}
                  onClick={() => exchange.mutate()}
                >
                  {t('Complete authorization')}
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
          </Field>
        </CredentialStep>
      )}
      <Field>
        <FieldLabel htmlFor={`${id}-files`}>
          {t('Import sub2api or CPA JSON files')}
        </FieldLabel>
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
        <FieldDescription className='text-xs'>
          {t(
            'Imported credentials are added to this form. Proxy settings and remote API addresses from backups are ignored.'
          )}
        </FieldDescription>
      </Field>
      {imported !== null && (
        <CredentialNotice tone='success'>
          {t('Imported {{count}} accounts. Save the channel to apply.', {
            count: imported,
          })}
        </CredentialNotice>
      )}
    </CredentialPanel>
  )
}

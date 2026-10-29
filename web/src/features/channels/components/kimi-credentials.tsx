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
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  cancelKimiOAuth,
  getKimiUsage,
  importKimiCredentials,
  pollKimiOAuth,
  startKimiOAuth,
  type AntigravityCredentialResult,
  type KimiDevice,
  type KimiPreview,
} from '../api'

type CredentialProps = {
  disabled: boolean
  proxy: string
  value: string
  channelId?: number
  onImport: (result: AntigravityCredentialResult) => void
}

function KimiDeviceLogin(
  props: CredentialProps & {
    region: string
    onBusyChange: (busy: boolean) => void
  }
) {
  const { t } = useTranslation()
  const onBusyChange = props.onBusyChange
  const isDisabled = props.disabled
  const onImport = props.onImport
  const [device, setDevice] = useState<KimiDevice | null>(null)
  const [message, setMessage] = useState('')
  const active = useRef(true)
  const disabled = useRef(props.disabled)
  disabled.current = props.disabled
  const handle = useRef('')
  const start = useMutation({
    mutationFn: () => startKimiOAuth(props.region, props.proxy),
    meta: { errorToast: false },
  })
  const { mutateAsync: pollToken, isPending: pollPending } = useMutation({
    mutationFn: pollKimiOAuth,
    meta: { errorToast: false },
  })
  const cancel = useMutation({ mutationFn: cancelKimiOAuth })
  useEffect(() => {
    onBusyChange(start.isPending || !!device || cancel.isPending)
    return () => onBusyChange(false)
  }, [start.isPending, device, cancel.isPending, onBusyChange])
  useEffect(() => {
    active.current = true
    return () => {
      active.current = false
      if (handle.current) void cancelKimiOAuth(handle.current).catch(() => {})
    }
  }, [])
  useEffect(() => {
    if (!device || isDisabled || pollPending) return
    const remaining = device.expires_at * 1000 - Date.now()
    const timer = setTimeout(
      () => {
        if (Date.now() >= device.expires_at * 1000) {
          setDevice(null)
          handle.current = ''
          setMessage('Kimi authorization expired; authorize again')
          return
        }
        void pollToken(device.handle)
          .then((result) => {
            const nextHandle =
              result.status === 'pending' ? result.handle : result.key
            if (!active.current || disabled.current) {
              if (active.current) {
                handle.current = ''
                setDevice(null)
              }
              void cancelKimiOAuth(nextHandle).catch(() => {})
              return
            }
            handle.current = nextHandle
            if (result.status === 'pending') {
              setDevice({
                ...device,
                handle: result.handle,
                interval: result.interval,
              })
            } else {
              onImport({ key: result.key, count: result.count })
              // The ready handle now belongs to the form until it is saved or expires.
              handle.current = ''
              setDevice(null)
              setMessage('Kimi authorization ready. Save the channel to apply.')
            }
          })
          .catch((error: unknown) => {
            if (!active.current) return
            setDevice(null)
            handle.current = ''
            handleServerError(error)
          })
      },
      Math.max(0, Math.min(device.interval * 1000, remaining))
    )
    return () => clearTimeout(timer)
  }, [device, isDisabled, pollToken, pollPending, onImport])
  const busy = props.disabled || start.isPending || cancel.isPending
  return (
    <div className='space-y-2'>
      <Button
        type='button'
        size='sm'
        disabled={busy || !!device}
        onClick={() => {
          setMessage('')
          void start
            .mutateAsync()
            .then((result) => {
              if (!active.current || disabled.current) {
                void cancelKimiOAuth(result.handle).catch(() => {})
                return
              }
              handle.current = result.handle
              setDevice(result)
            })
            .catch((error: unknown) => {
              if (active.current) handleServerError(error)
            })
        }}
      >
        {t('Sign in with Kimi Code')}
      </Button>
      {device && (
        <div className='space-y-2'>
          <a
            className='text-primary text-sm underline'
            href={device.verification_uri_complete || device.verification_uri}
            target='_blank'
            rel='noopener noreferrer'
          >
            {t('Open Kimi authorization')}
          </a>
          <div className='flex items-center gap-2'>
            <span>
              {t('Device code')}: <code>{device.user_code}</code>
            </span>
            <CopyButton
              value={device.user_code}
              size='sm'
              aria-label={t('Copy device code')}
            />
          </div>
          <p role='status' className='text-muted-foreground text-xs'>
            {t('Waiting for official authorization. Keep this form open.')}
          </p>
          <Button
            type='button'
            size='sm'
            variant='outline'
            disabled={busy || pollPending}
            onClick={() => {
              void cancel
                .mutateAsync(device.handle)
                .then(() => {
                  if (!active.current) return
                  handle.current = ''
                  setDevice(null)
                  setMessage('Authorization cancelled')
                })
                .catch(() => {})
            }}
          >
            {t('Cancel authorization')}
          </Button>
        </div>
      )}
      {message && (
        <p role='status' className='text-muted-foreground text-xs'>
          {t(message)}
        </p>
      )}
    </div>
  )
}

function KimiSubscription(props: { channelId: number }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const usage = useQuery({
    queryKey: ['channel', props.channelId, 'kimi-usage'],
    queryFn: () => getKimiUsage(props.channelId),
    enabled: false,
  })
  const labels: Record<string, string> = {
    limit_5h: 'Five-hour usage',
    limit_7d: 'Weekly usage',
    limit_month_total: 'Monthly total usage',
    limit_month_code: 'Monthly Coding usage',
  }
  return (
    <div className='space-y-2'>
      <Button
        type='button'
        size='sm'
        variant='outline'
        disabled={usage.isFetching}
        onClick={() => void usage.refetch()}
      >
        {t('Check Kimi subscription usage')}
      </Button>
      {usage.data?.length === 0 && (
        <p>{t('No subscription usage available')}</p>
      )}
      {usage.data?.map((entry) => (
        <p key={entry.name} className='text-muted-foreground text-xs'>
          {t(labels[entry.name] || entry.name)}:{' '}
          {formatNumber(entry.used_ratio * 100, locale)}%{' '}
          {entry.reset_time &&
            new Date(entry.reset_time).toLocaleString(locale)}
        </p>
      ))}
    </div>
  )
}

export function KimiCredentials(props: CredentialProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const id = useId()
  const [region, setRegion] = useState('mainland-cn')
  const [oauthBusy, setOAuthBusy] = useState(false)
  const [preview, setPreview] = useState<KimiPreview | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const active = useRef(true)
  const generation = useRef(0)
  useEffect(() => {
    active.current = true
    const activeRef = active
    const generationRef = generation
    return () => {
      activeRef.current = false
      generationRef.current++
    }
  }, [])
  const imported = useMutation({
    mutationFn: async (files: File[] | null) => {
      const current = generation.current
      let raw = props.value
      if (files) {
        if (
          files.length > 500 ||
          files.reduce((size, file) => size + file.size, 0) > 8 * 1024 * 1024
        ) {
          throw new Error(t('Kimi import exceeds size limit'))
        }
        const values: unknown[] = []
        for (const file of files) {
          const text = await file.text()
          try {
            values.push(JSON.parse(text) as unknown)
          } catch {
            for (const line of text.split('\n').filter((line) => line.trim())) {
              try {
                values.push(JSON.parse(line) as unknown)
              } catch {
                throw new Error(t('Invalid Kimi credential JSON'))
              }
            }
          }
        }
        raw = JSON.stringify(values)
      }
      return { result: await importKimiCredentials(raw, region), current }
    },
    onSuccess: ({ result, current }) => {
      if (!active.current || current !== generation.current) return
      setPreview(result)
      setSelected(new Set(result.accounts.map((account) => account.key)))
    },
  })
  const busy = props.disabled || imported.isPending || oauthBusy
  return (
    <div className='space-y-3 rounded-lg border p-3'>
      <p className='text-sm font-medium'>{t('Kimi Code credentials')}</p>
      <Label htmlFor={`${id}-region`}>{t('Kimi region')}</Label>
      <NativeSelect
        id={`${id}-region`}
        value={region}
        disabled={props.disabled}
        onChange={(event) => {
          generation.current++
          setRegion(event.target.value)
          setPreview(null)
          setSelected(new Set())
        }}
      >
        <NativeSelectOption value='mainland-cn'>
          {t('Mainland China (.com)')}
        </NativeSelectOption>
        <NativeSelectOption value='global'>
          {t('International (.ai)')}
        </NativeSelectOption>
      </NativeSelect>
      <KimiDeviceLogin
        key={`${region}:${props.proxy}`}
        {...props}
        disabled={props.disabled || imported.isPending}
        region={region}
        onBusyChange={setOAuthBusy}
      />
      <Button
        type='button'
        size='sm'
        variant='outline'
        disabled={busy || !props.value.trim()}
        onClick={() => imported.mutate(null)}
      >
        {t('Preview pasted Kimi JSON')}
      </Button>
      <Label htmlFor={`${id}-files`}>
        {t('Import sub2api or CPA JSON files')}
      </Label>
      <Input
        id={`${id}-files`}
        type='file'
        accept='.json,.jsonl,application/json'
        multiple
        disabled={busy}
        onChange={(event) => {
          const files = [...(event.target.files ?? [])]
          event.target.value = ''
          if (files.length) imported.mutate(files)
        }}
      />
      {preview && (
        <div className='space-y-2' aria-label={t('Kimi import preview')}>
          {preview.issues.map((issue) => (
            <p role='alert' key={issue.id} className='text-destructive text-xs'>
              {issue.name} {t(issue.error)}
            </p>
          ))}
          {preview.accounts.map((account, index) => (
            <Label key={account.key} className='flex items-center gap-2'>
              <Checkbox
                disabled={busy}
                checked={selected.has(account.key)}
                onCheckedChange={(checked) =>
                  setSelected((previous) => {
                    const next = new Set(previous)
                    if (checked) next.add(account.key)
                    else next.delete(account.key)
                    return next
                  })
                }
              />
              <span>
                {account.name || t('Account {{number}}', { number: index + 1 })}{' '}
                · {account.region} · {account.auth_type} ·{' '}
                {account.renewable
                  ? t('Renewable')
                  : t('Cannot renew automatically')}{' '}
                ·{' '}
                {account.expires_at
                  ? new Date(account.expires_at * 1000).toLocaleString(locale)
                  : t('Unknown expiry')}
              </span>
            </Label>
          ))}
          <Button
            type='button'
            size='sm'
            disabled={busy || selected.size === 0}
            onClick={() => {
              const accounts = preview.accounts.filter((account) =>
                selected.has(account.key)
              )
              props.onImport({
                key: accounts.map((account) => account.key).join('\n'),
                count: accounts.length,
              })
              setPreview(null)
              setSelected(new Set())
            }}
          >
            {t('Apply selected Kimi accounts')}
          </Button>
        </div>
      )}
      {props.channelId && <KimiSubscription channelId={props.channelId} />}
      <p className='text-muted-foreground text-xs'>
        {t(
          'Use Kimi Code OAuth or Coding credentials. Legacy web tokens require authorization again. PAYG keys belong in a Moonshot channel.'
        )}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Imported credentials are added to this form. Proxy settings and remote API addresses from backups are ignored.'
        )}
      </p>
    </div>
  )
}

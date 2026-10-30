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
import {
  CircleAlert,
  ClipboardPaste,
  KeyRound,
  LoaderCircle,
  LogIn,
  RefreshCw,
} from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Progress, ProgressLabel } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'

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
import {
  AuthorizationLink,
  CredentialNotice,
  CredentialPanel,
  CredentialStep,
  type CredentialNoticeTone,
} from './credential-panel'

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
  const [message, setMessage] = useState<{
    tone: CredentialNoticeTone
    text: string
  } | null>(null)
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
          setMessage({
            tone: 'warning',
            text: 'Kimi authorization expired; authorize again',
          })
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
              setMessage({
                tone: 'success',
                text: 'Kimi authorization ready. Save the channel to apply.',
              })
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
    <div className='flex flex-col gap-3'>
      <Button
        type='button'
        size='sm'
        className='self-start'
        disabled={busy || !!device}
        onClick={() => {
          setMessage(null)
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
        {start.isPending ? (
          <LoaderCircle className='animate-spin' aria-hidden='true' />
        ) : (
          <LogIn aria-hidden='true' />
        )}
        {t('Sign in with Kimi Code')}
      </Button>
      {device && (
        <CredentialStep>
          <div className='flex flex-col gap-1.5'>
            <span className='text-muted-foreground text-xs'>
              {t('Device code')}
            </span>
            <div className='flex items-center gap-2'>
              <code className='bg-background rounded-md border px-3 py-1.5 font-mono text-base font-semibold tracking-[0.2em]'>
                {device.user_code}
              </code>
              <CopyButton
                value={device.user_code}
                variant='outline'
                className='size-8'
                tooltip={t('Copy device code')}
                aria-label={t('Copy device code')}
              />
            </div>
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            <AuthorizationLink
              href={device.verification_uri_complete || device.verification_uri}
            >
              {t('Open Kimi authorization')}
            </AuthorizationLink>
            <Button
              type='button'
              size='sm'
              variant='ghost'
              disabled={busy || pollPending}
              onClick={() => {
                void cancel
                  .mutateAsync(device.handle)
                  .then(() => {
                    if (!active.current) return
                    handle.current = ''
                    setDevice(null)
                    setMessage({
                      tone: 'info',
                      text: 'Authorization cancelled',
                    })
                  })
                  .catch(() => {})
              }}
            >
              {t('Cancel authorization')}
            </Button>
          </div>
          <p
            role='status'
            className='text-muted-foreground flex items-center gap-2 text-xs'
          >
            <LoaderCircle
              className='size-3.5 animate-spin'
              aria-hidden='true'
            />
            {t('Waiting for official authorization. Keep this form open.')}
          </p>
        </CredentialStep>
      )}
      {message && (
        <CredentialNotice tone={message.tone}>
          {t(message.text)}
        </CredentialNotice>
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
    <div className='flex flex-col gap-3'>
      <div className='flex items-center justify-between gap-2'>
        <span className='text-sm font-medium'>{t('Usage')}</span>
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={usage.isFetching}
          onClick={() => void usage.refetch()}
        >
          <RefreshCw
            className={cn(usage.isFetching && 'animate-spin')}
            aria-hidden='true'
          />
          {t('Check Kimi subscription usage')}
        </Button>
      </div>
      {usage.data?.length === 0 && (
        <p className='text-muted-foreground text-xs'>
          {t('No subscription usage available')}
        </p>
      )}
      {!!usage.data?.length && (
        <div className='grid gap-3 sm:grid-cols-2'>
          {usage.data.map((entry) => {
            const percent = Math.min(100, Math.max(0, entry.used_ratio * 100))
            return (
              <Progress
                key={entry.name}
                value={percent}
                className='gap-1.5 rounded-lg border p-3'
              >
                <ProgressLabel className='flex-1 text-xs'>
                  {t(labels[entry.name] || entry.name)}
                </ProgressLabel>
                <span className='text-xs font-medium tabular-nums'>
                  {formatNumber(entry.used_ratio * 100, locale)}%
                </span>
                {entry.reset_time && (
                  <span className='text-muted-foreground order-last w-full text-xs'>
                    {t('Next reset')}:{' '}
                    {new Date(entry.reset_time).toLocaleString(locale)}
                  </span>
                )}
              </Progress>
            )
          })}
        </div>
      )}
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
  const regionLabels: Record<string, string> = {
    'mainland-cn': t('Mainland China (.com)'),
    global: t('International (.ai)'),
  }
  const authTypeLabels: Record<string, string> = {
    oauth: t('OAuth'),
    api_key: t('API Key'),
  }
  return (
    <CredentialPanel
      icon={<KeyRound />}
      title={t('Kimi Code credentials')}
      description={t(
        'Connect to Kimi Code with regional OAuth or Coding credentials'
      )}
    >
      <Field>
        <FieldLabel htmlFor={`${id}-region`}>{t('Kimi region')}</FieldLabel>
        <NativeSelect
          id={`${id}-region`}
          size='sm'
          className='w-full sm:w-56'
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
      </Field>
      <KimiDeviceLogin
        key={`${region}:${props.proxy}`}
        {...props}
        disabled={props.disabled || imported.isPending}
        region={region}
        onBusyChange={setOAuthBusy}
      />
      <Separator />
      <Field>
        <FieldLabel htmlFor={`${id}-files`}>
          {t('Import sub2api or CPA JSON files')}
        </FieldLabel>
        <div className='flex flex-col gap-2 sm:flex-row'>
          <Input
            id={`${id}-files`}
            type='file'
            accept='.json,.jsonl,application/json'
            multiple
            disabled={busy}
            className='sm:flex-1'
            onChange={(event) => {
              const files = [...(event.target.files ?? [])]
              event.target.value = ''
              if (files.length) imported.mutate(files)
            }}
          />
          <Button
            type='button'
            size='sm'
            variant='outline'
            className='h-8'
            disabled={busy || !props.value.trim()}
            onClick={() => imported.mutate(null)}
          >
            {imported.isPending ? (
              <LoaderCircle className='animate-spin' aria-hidden='true' />
            ) : (
              <ClipboardPaste aria-hidden='true' />
            )}
            {t('Preview pasted Kimi JSON')}
          </Button>
        </div>
        <FieldDescription className='text-xs'>
          {t(
            'Use Kimi Code OAuth or Coding credentials. Legacy web tokens require authorization again. PAYG keys belong in a Moonshot channel.'
          )}{' '}
          {t(
            'Imported credentials are added to this form. Proxy settings and remote API addresses from backups are ignored.'
          )}
        </FieldDescription>
      </Field>
      {preview && (
        <CredentialStep>
          <section
            aria-label={t('Kimi import preview')}
            className='flex flex-col gap-3'
          >
            {preview.issues.length > 0 && (
              <Alert variant='destructive'>
                <CircleAlert aria-hidden='true' />
                <AlertDescription>
                  <ul className='flex flex-col gap-1'>
                    {preview.issues.map((issue) => (
                      <li key={issue.id} className='break-all'>
                        <span className='font-medium'>{issue.name}</span>{' '}
                        {t(issue.error)}
                      </li>
                    ))}
                  </ul>
                </AlertDescription>
              </Alert>
            )}
            {preview.accounts.length > 0 && (
              <ul className='bg-background divide-border/60 divide-y rounded-lg border'>
                {preview.accounts.map((account, index) => (
                  <li key={account.key}>
                    <Label className='hover:bg-muted/50 flex cursor-pointer items-start gap-3 px-3 py-2.5 font-normal'>
                      <Checkbox
                        className='mt-0.5'
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
                      <span className='flex min-w-0 flex-1 flex-col gap-1.5'>
                        <span className='truncate text-sm font-medium'>
                          {account.name ||
                            t('Account {{number}}', { number: index + 1 })}
                        </span>
                        <span className='flex flex-wrap items-center gap-1.5'>
                          <Badge variant='secondary'>
                            {regionLabels[account.region] || account.region}
                          </Badge>
                          <Badge variant='outline'>
                            {authTypeLabels[account.auth_type] ||
                              account.auth_type}
                          </Badge>
                          <Badge
                            variant={account.renewable ? 'outline' : 'warning'}
                          >
                            {account.renewable
                              ? t('Renewable')
                              : t('Cannot renew automatically')}
                          </Badge>
                          <span className='text-muted-foreground text-xs'>
                            {account.expires_at
                              ? t('Expires {{time}}', {
                                  time: new Date(
                                    account.expires_at * 1000
                                  ).toLocaleString(locale),
                                })
                              : t('Unknown expiry')}
                          </span>
                        </span>
                      </span>
                    </Label>
                  </li>
                ))}
              </ul>
            )}
            <div className='flex items-center justify-between gap-2'>
              <span className='text-muted-foreground text-xs'>
                {t('{{count}} selected', { count: selected.size })}
              </span>
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
          </section>
        </CredentialStep>
      )}
      {props.channelId && (
        <>
          <Separator />
          <KimiSubscription channelId={props.channelId} />
        </>
      )}
    </CredentialPanel>
  )
}

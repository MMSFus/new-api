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

import { CopyButton } from '@/components/copy-button'
import { useStatus } from '@/hooks/use-status'

export function QuickStart() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const serverAddress =
    (typeof status?.server_address === 'string' &&
      status.server_address.trim().replace(/\/+$/, '')) ||
    window.location.origin
  const baseUrl = `${serverAddress}/v1`

  const steps = [
    {
      title: t('Create an account'),
      desc: t('Sign up and top up your balance.'),
    },
    {
      title: t('Create an API key'),
      desc: t('Open API Keys in the console and create a key.'),
    },
    {
      title: t('Replace the base URL'),
      desc: t(
        'Point your app or SDK at the address below and keep the rest of your code.'
      ),
    },
  ]

  return (
    <section className='px-4 py-14 sm:px-6 md:py-20'>
      <div className='mx-auto max-w-6xl'>
        <h2 className='text-2xl font-bold tracking-tight text-balance md:text-3xl'>
          {t('Get started in three steps')}
        </h2>
        <ol className='mt-8 grid grid-cols-1 gap-6 md:grid-cols-3'>
          {steps.map((step, index) => (
            <li key={step.title} className='flex gap-4'>
              <span
                className='border-border text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-full border text-sm font-semibold tabular-nums'
                aria-hidden='true'
              >
                {index + 1}
              </span>
              <div className='min-w-0 space-y-1'>
                <h3 className='font-semibold'>{step.title}</h3>
                <p className='text-muted-foreground text-sm leading-relaxed'>
                  {step.desc}
                </p>
              </div>
            </li>
          ))}
        </ol>

        <div className='bg-muted/40 border-border/60 mt-8 flex items-center gap-2 rounded-lg border py-2 ps-4 pe-2'>
          <span className='text-muted-foreground shrink-0 text-xs'>
            {t('Base URL')}
          </span>
          <code className='min-w-0 flex-1 font-mono text-sm break-all'>
            {baseUrl}
          </code>
          <CopyButton
            value={baseUrl}
            variant='ghost'
            size='icon'
            aria-label={t('Copy base URL')}
          />
        </div>
      </div>
    </section>
  )
}

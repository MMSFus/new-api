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
import { Loader2, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type MomentsAuthEntryProps = {
  variant: 'sign-in' | 'sign-up'
  displayName?: string
  onContinue: () => void
  disabled?: boolean
  loading?: boolean
  className?: string
}

export function MomentsAuthEntry(props: MomentsAuthEntryProps) {
  const { t } = useTranslation()
  const displayName = props.displayName?.trim() || 'OIDC'
  const isSignUp = props.variant === 'sign-up'
  const buttonLabel = isSignUp
    ? t('Register or sign in with {{name}}', { name: displayName })
    : t('Sign in or register with {{name}}', { name: displayName })
  const loadingLabel = t('Connecting to {{name}}...', { name: displayName })
  const description = isSignUp
    ? t(
        'Your New API account will be created automatically after you continue. You do not need to set another username or password here.'
      )
    : t(
        'New users and {{name}} users should choose this option. Your account will be created automatically the first time you use it.',
        { name: displayName }
      )

  return (
    <section
      aria-label={t('{{name}} authentication', { name: displayName })}
      className={cn(
        'border-primary/30 bg-primary/5 space-y-4 rounded-xl border p-4',
        props.className
      )}
    >
      <div className='flex items-start gap-3'>
        <div className='bg-primary/10 text-primary mt-0.5 flex size-10 shrink-0 items-center justify-center rounded-lg'>
          <ShieldCheck className='size-5' aria-hidden='true' />
        </div>
        <div className='min-w-0 flex-1 space-y-1'>
          <div className='flex flex-wrap items-center gap-2'>
            <p className='font-semibold'>{displayName}</p>
            <Badge variant='secondary'>{t('Recommended')}</Badge>
          </div>
          <p className='text-muted-foreground text-sm'>{description}</p>
        </div>
      </div>

      <Button
        type='button'
        className='w-full justify-center gap-2'
        disabled={props.disabled || props.loading}
        onClick={props.onContinue}
      >
        {props.loading ? (
          <Loader2 className='size-4 animate-spin' aria-hidden='true' />
        ) : (
          <ShieldCheck className='size-4' aria-hidden='true' />
        )}
        {props.loading ? loadingLabel : buttonLabel}
      </Button>
    </section>
  )
}

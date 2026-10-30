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
import { CircleAlert, CircleCheck, ExternalLink, Info } from 'lucide-react'
import type { ReactNode } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { cn } from '@/lib/utils'

/** Card shell shared by provider credential helpers inside the auth section. */
export function CredentialPanel(props: {
  icon: ReactNode
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <div className='border-border/60 flex flex-col gap-4 rounded-lg border p-4'>
      <div className='flex items-start gap-3'>
        <IconBadge tone='primary' size='md'>
          {props.icon}
        </IconBadge>
        <div className='min-w-0 flex-1'>
          <h3 className='text-sm font-semibold tracking-tight'>
            {props.title}
          </h3>
          <p className='text-muted-foreground mt-1 text-xs leading-5'>
            {props.description}
          </p>
        </div>
      </div>
      {props.children}
    </div>
  )
}

/** Inset area for an in-progress authorization step. */
export function CredentialStep(props: {
  className?: string
  children: ReactNode
}) {
  return (
    <div
      className={cn(
        'bg-muted/40 border-border/60 flex flex-col gap-3 rounded-lg border p-3',
        props.className
      )}
    >
      {props.children}
    </div>
  )
}

export function AuthorizationLink(props: {
  href: string
  children: ReactNode
}) {
  return (
    <Button
      role='link'
      variant='outline'
      size='sm'
      render={<a href={props.href} target='_blank' rel='noopener noreferrer' />}
    >
      <ExternalLink aria-hidden='true' />
      {props.children}
    </Button>
  )
}

export type CredentialNoticeTone = 'success' | 'warning' | 'info'

const noticeIcons = {
  success: CircleCheck,
  warning: CircleAlert,
  info: Info,
} as const

/** Polite status message; errors that need attention use a destructive Alert. */
export function CredentialNotice(props: {
  tone: CredentialNoticeTone
  children: ReactNode
}) {
  const Icon = noticeIcons[props.tone]
  return (
    <Alert
      role='status'
      className={cn(
        props.tone === 'success' && 'border-success/30 text-success',
        props.tone === 'warning' && 'border-warning/40 text-warning'
      )}
    >
      <Icon aria-hidden='true' />
      <AlertDescription className='text-foreground'>
        {props.children}
      </AlertDescription>
    </Alert>
  )
}

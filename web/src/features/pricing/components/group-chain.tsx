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
import { GroupBadge } from '@/components/group-badge'
import { cn } from '@/lib/utils'

/** Groups in the order requests try them, joined by arrows. */
export function GroupChain(props: {
  label: string
  groups: string[]
  className?: string
}) {
  if (props.groups.length === 0) return null

  return (
    <div
      className={cn(
        'text-muted-foreground flex flex-wrap items-center gap-1 text-xs',
        props.className
      )}
    >
      <span className='font-medium'>{props.label}</span>
      <span className='text-muted-foreground/40' aria-hidden='true'>
        →
      </span>
      {props.groups.map((g, idx) => (
        <span key={g} className='flex items-center gap-1'>
          <GroupBadge group={g} size='sm' />
          {idx < props.groups.length - 1 && (
            <span className='text-muted-foreground/40' aria-hidden='true'>
              →
            </span>
          )}
        </span>
      ))}
    </div>
  )
}

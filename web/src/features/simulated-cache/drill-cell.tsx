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
import { Button } from '@/components/ui/button'

/** Table cell that opens its channel or model in the drill sheet. */
export function DrillCell(props: {
  label: string
  title: string
  onClick?: () => void
}) {
  if (!props.onClick) return <span className='truncate'>{props.label}</span>
  return (
    <Button
      type='button'
      variant='link'
      size='sm'
      className='h-auto max-w-full justify-start truncate p-0 font-normal'
      title={props.title}
      onClick={props.onClick}
    >
      {props.label}
    </Button>
  )
}

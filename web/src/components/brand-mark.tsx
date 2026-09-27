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
import { cn } from '@/lib/utils'

// Six spokes curving the same way into the center: many models swirling
// into one endpoint. Drawn on a 48-unit grid; keep in sync with
// public/favicon.svg.
const SPOKES_PATH =
  'M24 3 Q32.22 8.55 32.27 17.54 M42.19 13.5 Q41.49 23.39 33.74 27.93 ' +
  'M42.19 34.5 Q33.27 38.84 25.46 34.4 M24 45 Q15.78 39.45 15.73 30.46 ' +
  'M5.81 34.5 Q6.51 24.61 14.26 20.07 M5.81 13.5 Q14.73 9.16 22.54 13.6'

type BrandMarkProps = {
  /**
   * - `plain`: spokes in the current text color, hub in the theme accent.
   * - `tile`: the whole mark on an accent-filled rounded tile.
   */
  variant?: 'plain' | 'tile'
  className?: string
}

/**
 * The built-in site mark, shown whenever the administrator has not
 * configured a custom logo. Colors come from theme tokens so it follows the
 * active color theme and light/dark mode.
 */
export function BrandMark(props: BrandMarkProps) {
  const variant = props.variant ?? 'plain'
  const tile = variant === 'tile'
  return (
    <span
      data-slot='brand-mark'
      aria-hidden='true'
      className={cn(
        'inline-flex shrink-0 items-center justify-center',
        tile && 'bg-primary text-primary-foreground rounded-[28%]',
        props.className
      )}
    >
      <svg
        viewBox='0 0 48 48'
        className={cn(tile ? 'size-[72%]' : 'size-full')}
        focusable='false'
      >
        <path
          d={SPOKES_PATH}
          fill='none'
          stroke='currentColor'
          strokeWidth={3.8}
          strokeLinecap='round'
        />
        <circle
          cx={24}
          cy={24}
          r={6.5}
          className={cn(tile ? 'fill-current' : 'fill-primary')}
        />
      </svg>
    </span>
  )
}

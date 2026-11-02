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

export interface MetricGridItem {
  label: string
  value: string
  /** Tone of the value, e.g. a warning colour. */
  className?: string
}

/** Totals of a drilled slice as a compact label/value grid. */
export function MetricGrid(props: { metrics: MetricGridItem[] }) {
  return (
    <dl className='grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-4'>
      {props.metrics.map((metric) => (
        <div
          key={metric.label}
          className='bg-muted/40 min-w-0 rounded-lg px-3 py-2'
        >
          <dt className='text-muted-foreground truncate text-xs'>
            {metric.label}
          </dt>
          <dd
            className={cn(
              'mt-1 truncate font-mono text-sm font-semibold tabular-nums',
              metric.className
            )}
            title={metric.value}
          >
            {metric.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}

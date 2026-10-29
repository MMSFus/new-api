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
import type { Table } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from '@/features/usage-logs/components/logs-filter-toolbar'

import type { SimulatedCacheFilters } from './api'

/** The log range search shared with the cost report, which adds a group. */
type LogRangeFilters = SimulatedCacheFilters & { group?: string }

export function SimulatedCacheFilterBar<TData>(props: {
  table: Table<TData>
  filters: LogRangeFilters
  onChange: (patch: Partial<LogRangeFilters>) => void
  /** Shows the group filter. */
  withGroup?: boolean
  isFetching: boolean
  onSearch: () => void
  onReset: () => void
}) {
  const { t } = useTranslation()
  const dateFilter = (
    <LogsFilterField wide>
      <CompactDateTimeRangePicker
        start={
          props.filters.start_timestamp === undefined
            ? undefined
            : new Date(props.filters.start_timestamp * 1000)
        }
        end={
          props.filters.end_timestamp === undefined
            ? undefined
            : new Date(props.filters.end_timestamp * 1000)
        }
        onChange={({ start, end }) =>
          props.onChange({
            start_timestamp: start
              ? Math.floor(start.getTime() / 1000)
              : undefined,
            end_timestamp: end ? Math.floor(end.getTime() / 1000) : undefined,
          })
        }
      />
    </LogsFilterField>
  )
  const textFilters = (
    <>
      <LogsFilterField>
        <LogsFilterInput
          aria-label={t('Channel ID')}
          placeholder={t('Channel ID')}
          inputMode='numeric'
          value={props.filters.channel ?? ''}
          onChange={(event) => {
            const channel = Number.parseInt(event.target.value, 10)
            props.onChange({
              channel:
                Number.isFinite(channel) && channel > 0 ? channel : undefined,
            })
          }}
        />
      </LogsFilterField>
      <LogsFilterField>
        <LogsFilterInput
          aria-label={t('Model Name')}
          placeholder={t('Model Name')}
          value={props.filters.model_name ?? ''}
          onChange={(event) =>
            props.onChange({ model_name: event.target.value || undefined })
          }
        />
      </LogsFilterField>
      <LogsFilterField>
        <LogsFilterInput
          aria-label={t('Username')}
          placeholder={t('Username')}
          value={props.filters.username ?? ''}
          onChange={(event) =>
            props.onChange({ username: event.target.value || undefined })
          }
        />
      </LogsFilterField>
      {props.withGroup && (
        <LogsFilterField>
          <LogsFilterInput
            aria-label={t('Group')}
            placeholder={t('Group')}
            value={props.filters.group ?? ''}
            onChange={(event) =>
              props.onChange({ group: event.target.value || undefined })
            }
          />
        </LogsFilterField>
      )}
    </>
  )
  const filterCount = [
    props.filters.channel,
    props.filters.model_name,
    props.filters.username,
    props.filters.group,
  ].filter(Boolean).length
  return (
    <LogsFilterToolbar
      table={props.table}
      primaryFilters={
        <>
          {dateFilter}
          {textFilters}
        </>
      }
      mobilePinnedFilters={dateFilter}
      mobileFilters={textFilters}
      mobileFilterCount={filterCount}
      hasActiveFilters
      searchLoading={props.isFetching}
      onSearch={props.onSearch}
      onReset={props.onReset}
    />
  )
}

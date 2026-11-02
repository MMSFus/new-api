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

import { SimulatedCacheFilterBar } from '@/features/simulated-cache/filter-bar'
import {
  LogsFilterField,
  LogsFilterInput,
} from '@/features/usage-logs/components/logs-filter-toolbar'

import type { SessionRecorderTab } from './lib'
import type { SessionRecorderSearchState } from './use-session-recorder-search'

const QUICK_RANGES = ['24h', '7d', '30d'] as const

/**
 * Range and model filters of both tabs; the content tab adds the user and
 * session filters of the session list.
 */
export function SessionRecorderFilterBar(props: {
  search: SessionRecorderSearchState
  tab: SessionRecorderTab
  isFetching: boolean
  onSearch: () => void
}) {
  const { t } = useTranslation()
  const search = props.search
  const range = search.range
  const filters = search.sessionFilters
  const content = props.tab === 'content'
  const textFilters = (
    <>
      <LogsFilterField>
        <LogsFilterInput
          aria-label={t('Model Name')}
          placeholder={t('Model Name')}
          value={range.filters.model_name ?? ''}
          onChange={(event) =>
            range.updateFilters({ model_name: event.target.value || undefined })
          }
        />
      </LogsFilterField>
      {content && (
        <>
          <LogsFilterField>
            <LogsFilterInput
              aria-label={t('User ID')}
              placeholder={t('User ID')}
              inputMode='numeric'
              value={filters.user_id ?? ''}
              onChange={(event) => {
                const user = Number.parseInt(event.target.value, 10)
                search.updateSessionFilters({
                  user_id: Number.isFinite(user) && user > 0 ? user : undefined,
                })
              }}
            />
          </LogsFilterField>
          <LogsFilterField>
            <LogsFilterInput
              aria-label={t('Session ID')}
              placeholder={t('Session ID')}
              value={filters.session_id ?? ''}
              onChange={(event) =>
                search.updateSessionFilters({
                  session_id: event.target.value || undefined,
                })
              }
            />
          </LogsFilterField>
        </>
      )}
    </>
  )
  const activeCount = [
    filters.model,
    content ? filters.user_id : undefined,
    content ? filters.session_id : undefined,
  ].filter(Boolean).length

  return (
    <SimulatedCacheFilterBar
      filters={range.filters}
      onChange={range.updateFilters}
      textFilters={textFilters}
      textFilterCount={activeCount}
      isFetching={props.isFetching}
      onSearch={props.onSearch}
      onReset={search.reset}
      presets={QUICK_RANGES}
      preset={range.preset}
      onPresetChange={range.selectPreset}
    />
  )
}

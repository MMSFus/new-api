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
import { Layers } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { PricingConfigGroup } from '../types'
import { GroupChain } from './group-chain'

/** Describes the selected config group above the filtered model list. */
export function ConfigGroupSummary(props: {
  configGroup: PricingConfigGroup
  modelCount: number
}) {
  const { t } = useTranslation()

  return (
    <section
      aria-label={t('Config Groups')}
      className='bg-card space-y-2 rounded-xl border p-3'
    >
      <div className='flex flex-wrap items-center gap-2'>
        <Layers className='text-muted-foreground size-4' aria-hidden='true' />
        <h2 className='text-foreground text-sm font-semibold'>
          {props.configGroup.name || props.configGroup.key}
        </h2>
        <span className='text-muted-foreground text-xs'>
          {t('{{count}} models reachable', { count: props.modelCount })}
        </span>
      </div>
      {props.configGroup.description && (
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {props.configGroup.description}
        </p>
      )}
      <GroupChain
        label={t('Member groups')}
        groups={props.configGroup.groups}
      />
    </section>
  )
}

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

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { CHANNEL_COST_DIMENSIONS, type ChannelCostDimension } from './lib'
import { useDimensionLabels } from './use-dimension-labels'

/** Switches the dimension shared by the revenue share chart and the table. */
export function ChannelCostDimensionTabs(props: {
  value: ChannelCostDimension
  onValueChange: (dimension: ChannelCostDimension) => void
}) {
  const { t } = useTranslation()
  const labels = useDimensionLabels()
  return (
    <Tabs
      value={props.value}
      onValueChange={(value) =>
        props.onValueChange(value as ChannelCostDimension)
      }
    >
      <TabsList aria-label={t('Breakdown dimension')}>
        {CHANNEL_COST_DIMENSIONS.map((value) => (
          <TabsTrigger key={value} value={value}>
            {labels[value]}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}

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
import { useWatch, type UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { AutoGroupOrderItem } from '@/components/auto-group-order-item'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import { REVENUE_MATCH_TYPES, type RevenueMatchType } from '../api'
import type { RevenueRulesFormValues } from './rules-schema'

const MATCH_TYPE_LABELS: Record<RevenueMatchType, string> = {
  keyword: 'Keyword (contains, case-insensitive)',
  regex: 'Regular expression (RE2)',
  glob: 'Glob (exact, prefix* or wildcards)',
}

const PATTERN_PLACEHOLDERS: Record<RevenueMatchType, string> = {
  keyword: 'gpt',
  regex: '^gemini-3(\\.\\d+)?-',
  glob: 'claude-opus-*',
}

/** One draggable rule of the revenue rule editor. */
export function RevenueRuleRow(props: {
  form: UseFormReturn<RevenueRulesFormValues>
  index: number
  count: number
  /** Reorder value of the row. */
  rowKey: string
  unit: string
  onMove: (index: number, direction: 'up' | 'down') => void
  onRemove: (index: number) => void
}) {
  const { t } = useTranslation()
  const index = props.index
  const name = useWatch({
    control: props.form.control,
    name: `rules.${index}.name`,
  })
  const matchType = useWatch({
    control: props.form.control,
    name: `rules.${index}.match_type`,
  })
  const label = name?.trim() || t('Rule {{number}}', { number: index + 1 })

  return (
    <AutoGroupOrderItem
      group={props.rowKey}
      label={label}
      index={index}
      count={props.count}
      onMove={props.onMove}
      onRemove={() => props.onRemove(index)}
      leading={
        <span className='text-muted-foreground w-5 shrink-0 text-center text-sm tabular-nums'>
          {index + 1}
        </span>
      }
    >
      <div className='grid w-full gap-2 sm:grid-cols-2 lg:grid-cols-[1fr_1.2fr_1.6fr_0.8fr_auto]'>
        <FormField
          control={props.form.control}
          name={`rules.${index}.name`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className='text-xs'>{t('Rule name')}</FormLabel>
              <FormControl>
                <Input {...field} />
              </FormControl>
              <FormMessage className='text-xs' />
            </FormItem>
          )}
        />
        <FormField
          control={props.form.control}
          name={`rules.${index}.match_type`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className='text-xs'>{t('Match type')}</FormLabel>
              <Select
                items={REVENUE_MATCH_TYPES.map((type) => ({
                  value: type,
                  label: t(MATCH_TYPE_LABELS[type]),
                }))}
                value={field.value}
                onValueChange={(value) => {
                  if (value) field.onChange(value)
                }}
              >
                <FormControl>
                  <SelectTrigger className='w-full'>
                    <SelectValue />
                  </SelectTrigger>
                </FormControl>
                <SelectContent alignItemWithTrigger={false}>
                  {REVENUE_MATCH_TYPES.map((type) => (
                    <SelectItem key={type} value={type}>
                      {t(MATCH_TYPE_LABELS[type])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage className='text-xs' />
            </FormItem>
          )}
        />
        <FormField
          control={props.form.control}
          name={`rules.${index}.pattern`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className='text-xs'>{t('Expression')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  className='font-mono'
                  placeholder={PATTERN_PLACEHOLDERS[matchType ?? 'keyword']}
                />
              </FormControl>
              <FormMessage className='text-xs' />
            </FormItem>
          )}
        />
        <FormField
          control={props.form.control}
          name={`rules.${index}.price_per_tb`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className='text-xs'>
                {t('Price per TB ({{unit}})', { unit: props.unit })}
              </FormLabel>
              <FormControl>
                <Input {...field} inputMode='decimal' className='font-mono' />
              </FormControl>
              <FormMessage className='text-xs' />
            </FormItem>
          )}
        />
        <FormField
          control={props.form.control}
          name={`rules.${index}.enabled`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className='text-xs'>{t('Enabled')}</FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
            </FormItem>
          )}
        />
      </div>
    </AutoGroupOrderItem>
  )
}

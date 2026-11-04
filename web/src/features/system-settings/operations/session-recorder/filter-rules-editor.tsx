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
import { Plus, Trash2 } from 'lucide-react'
import { useMemo } from 'react'
import {
  useFieldArray,
  useWatch,
  type Control,
  type UseFormReturn,
} from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  ComboboxInput,
  type ComboboxInputOption,
} from '@/components/ui/combobox-input'
import {
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form'

import {
  useRecorderChannels,
  useRecorderGroups,
  useRecorderModels,
  type RecorderChannelOption,
} from './api'
import { EMPTY_RULE, type RecorderFormValues } from './lib/settings-schema'

type RuleListName = 'filter.include' | 'filter.exclude'

type FilterRulesEditorProps = {
  form: UseFormReturn<RecorderFormValues>
  name: RuleListName
  title: string
  description: string
  disabled?: boolean
}

type RuleRowProps = {
  control: Control<RecorderFormValues>
  name: RuleListName
  index: number
  channels: RecorderChannelOption[]
  channelOptions: ComboboxInputOption[]
  groupOptions: ComboboxInputOption[]
  allModelOptions: ComboboxInputOption[]
  disabled?: boolean
  onChange: (
    index: number,
    field: 'channel_id' | 'group' | 'model',
    value: string
  ) => void
  onRemove: (index: number) => void
}

function toOptions(values: string[]): ComboboxInputOption[] {
  return values.map((value) => ({ value, label: value }))
}

function RuleRow(props: RuleRowProps) {
  const { t } = useTranslation()
  const rule = useWatch({
    control: props.control,
    name: `${props.name}.${props.index}`,
  })
  const channelId = rule?.channel_id ?? 0
  const channelSearch = rule?.channel_search ?? ''
  // A selected channel narrows the model list to that channel's models.
  const modelOptions = useMemo(() => {
    if (channelId <= 0) return props.allModelOptions
    const channel = props.channels.find((c) => c.id === channelId)
    return channel ? toOptions(channel.models) : props.allModelOptions
  }, [channelId, props.channels, props.allModelOptions])

  return (
    <div className='grid grid-cols-1 items-center gap-2 md:grid-cols-[1fr_1fr_1fr_auto]'>
      <FormField
        control={props.control}
        name={`${props.name}.${props.index}.channel_search`}
        render={({ field, fieldState }) => (
          <FormItem className='gap-1'>
            <FormControl>
              <ComboboxInput
                aria-label={t('Channel')}
                aria-invalid={fieldState.invalid}
                options={props.channelOptions}
                value={
                  channelSearch || (channelId > 0 ? String(channelId) : '')
                }
                onSearchValueChange={field.onChange}
                onValueChange={(value) =>
                  props.onChange(props.index, 'channel_id', value)
                }
                placeholder={t('Any channel')}
                onKeyDown={(event) => {
                  if (event.key !== 'Enter') return
                  event.preventDefault()
                  const value = event.currentTarget.value
                    .trim()
                    .replace(/^#/, '')
                  if (
                    /^\d+$/.test(value) &&
                    Number.isSafeInteger(Number(value)) &&
                    Number(value) > 0
                  ) {
                    props.onChange(props.index, 'channel_id', value)
                  }
                }}
                disabled={props.disabled}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <ComboboxInput
        aria-label={t('Group')}
        options={props.groupOptions}
        value={rule?.group ?? ''}
        onValueChange={(value) => props.onChange(props.index, 'group', value)}
        placeholder={t('Any group')}
        allowCustomValue
        disabled={props.disabled}
      />
      <ComboboxInput
        aria-label={t('Model')}
        options={modelOptions}
        value={rule?.model ?? ''}
        onValueChange={(value) => props.onChange(props.index, 'model', value)}
        placeholder={t('Any model (supports prefix* and wildcards)')}
        allowCustomValue
        disabled={props.disabled}
      />
      <Button
        type='button'
        variant='ghost'
        size='icon'
        aria-label={t('Remove rule')}
        onClick={() => props.onRemove(props.index)}
        disabled={props.disabled}
      >
        <Trash2 className='size-4' />
      </Button>
    </div>
  )
}

export function FilterRulesEditor(props: FilterRulesEditorProps) {
  const { t } = useTranslation()
  const rules = useFieldArray({ control: props.form.control, name: props.name })
  const channelsQuery = useRecorderChannels()
  const groupsQuery = useRecorderGroups()
  const modelsQuery = useRecorderModels()

  const channels = useMemo(() => channelsQuery.data ?? [], [channelsQuery.data])
  const channelOptions = useMemo(
    () => [
      { value: '', label: t('Any channel') },
      ...channels.map((c) => ({
        value: String(c.id),
        label: `#${c.id} ${c.name}`,
      })),
    ],
    [channels, t]
  )
  const groupOptions = useMemo(
    () => toOptions(groupsQuery.data ?? []),
    [groupsQuery.data]
  )
  const allModelOptions = useMemo(
    () => toOptions(modelsQuery.data ?? []),
    [modelsQuery.data]
  )

  const handleChange = (
    index: number,
    field: 'channel_id' | 'group' | 'model',
    value: string
  ) => {
    const options = { shouldDirty: true, shouldValidate: true }
    if (field === 'channel_id') {
      props.form.setValue(`${props.name}.${index}.channel_search`, '', options)
      const text = value.replace(/^#/, '')
      const parsed = /^\d+$/.test(text) ? Number(text) : 0
      if (value !== '' && (!Number.isSafeInteger(parsed) || parsed <= 0)) return
      props.form.setValue(
        `${props.name}.${index}.channel_id`,
        Number.isFinite(parsed) && parsed > 0 ? parsed : 0,
        options
      )
      return
    }
    props.form.setValue(`${props.name}.${index}.${field}`, value, options)
  }

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-2'>
        <div>
          <h5 className='text-sm font-medium'>{props.title}</h5>
          <p className='text-muted-foreground text-xs'>{props.description}</p>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => rules.append({ ...EMPTY_RULE })}
          disabled={props.disabled}
        >
          <Plus className='size-4' />
          {t('Add rule')}
        </Button>
      </div>
      {rules.fields.length === 0 && (
        <p className='text-muted-foreground text-xs'>{t('No rules')}</p>
      )}
      {rules.fields.map((field, index) => (
        <RuleRow
          key={field.id}
          control={props.form.control}
          name={props.name}
          index={index}
          channels={channels}
          channelOptions={channelOptions}
          groupOptions={groupOptions}
          allModelOptions={allModelOptions}
          disabled={props.disabled}
          onChange={handleChange}
          onRemove={rules.remove}
        />
      ))}
    </div>
  )
}

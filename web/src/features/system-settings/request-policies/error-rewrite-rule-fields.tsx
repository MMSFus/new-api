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
import { Trash2 } from 'lucide-react'
import type { Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsFormGrid,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import type {
  ErrorRewriteFormValues,
  ErrorRewriteRuleFormValue,
} from './error-rewrite-form'

type TextFieldName = Exclude<
  keyof ErrorRewriteRuleFormValue,
  'enabled' | 'skip_retry'
>

type ErrorRewriteRuleFieldsProps = {
  control: Control<ErrorRewriteFormValues>
  index: number
  onRemove: (index: number) => void
}

type RuleTextFieldProps = {
  control: Control<ErrorRewriteFormValues>
  index: number
  name: TextFieldName
  label: string
  description?: string
  placeholder?: string
  multiline?: boolean
}

function RuleTextField(props: RuleTextFieldProps) {
  return (
    <FormField
      control={props.control}
      name={`rules.${props.index}.${props.name}`}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            {props.multiline ? (
              <Textarea rows={3} placeholder={props.placeholder} {...field} />
            ) : (
              <Input placeholder={props.placeholder} {...field} />
            )}
          </FormControl>
          {props.description ? (
            <FormDescription>{props.description}</FormDescription>
          ) : null}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function ErrorRewriteRuleFields(props: ErrorRewriteRuleFieldsProps) {
  const { t } = useTranslation()
  const prefix = `error-rewrite-rule-${props.index}`
  const common = { control: props.control, index: props.index }

  return (
    <div className='space-y-4 rounded-lg border p-4'>
      <div className='flex items-center justify-between gap-2'>
        <h4 className='text-sm font-medium'>
          {t('Rule {{index}}', { index: props.index + 1 })}
        </h4>
        <Button
          type='button'
          variant='ghost'
          size='icon'
          aria-label={t('Delete rule')}
          onClick={() => props.onRemove(props.index)}
        >
          <Trash2 className='size-4' aria-hidden='true' />
        </Button>
      </div>
      <SettingsFormGrid>
        <FormField
          control={props.control}
          name={`rules.${props.index}.enabled`}
          render={({ field }) => (
            <SettingsSwitchField
              controlId={`${prefix}-enabled`}
              checked={field.value}
              onCheckedChange={field.onChange}
              label={t('Enabled')}
            />
          )}
        />
        <RuleTextField {...common} name='name' label={t('Name')} />
        <RuleTextField
          {...common}
          name='channel_ids'
          label={t('Channel IDs')}
          description={t('Leave empty to match all channels.')}
          placeholder='1, 2'
        />
        <RuleTextField
          {...common}
          name='status_codes'
          label={t('Upstream status codes')}
          placeholder='400,500-599'
        />
        <RuleTextField
          {...common}
          name='error_codes'
          label={t('Upstream error codes or types')}
          description={t('Separate multiple values with commas.')}
          placeholder='upstream_extra_usage_required'
        />
        <RuleTextField
          {...common}
          name='keywords'
          label={t('Keywords')}
          description={t(
            'One keyword per line. Any keyword matches, ignoring case.'
          )}
          multiline
        />
        <RuleTextField
          {...common}
          name='message_regex'
          label={t('Message regular expression')}
        />
        <RuleTextField
          {...common}
          name='response_status_code'
          label={t('Customer status code')}
          description={t('Leave empty to keep the original status code.')}
          placeholder='403'
        />
        <RuleTextField
          {...common}
          name='response_error_code'
          label={t('Customer error code')}
          placeholder='third_party_client_not_supported'
        />
        <RuleTextField
          {...common}
          name='response_message'
          label={t('Customer message')}
          multiline
        />
        <FormField
          control={props.control}
          name={`rules.${props.index}.skip_retry`}
          render={({ field }) => (
            <SettingsSwitchField
              controlId={`${prefix}-skip-retry`}
              checked={field.value}
              onCheckedChange={field.onChange}
              label={t('Stop retrying')}
              description={t(
                'When matched, do not retry the request on other channels.'
              )}
            />
          )}
        />
      </SettingsFormGrid>
    </div>
  )
}

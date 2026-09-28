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
import type { Control, FieldPath, FieldPathValue } from 'react-hook-form'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import {
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../../components/settings-form-layout'
import { safeNumberFieldProps } from '../../utils/numeric-field'
import type { RecorderFormValues } from './lib/settings-schema'

type Paths<T> = {
  [K in FieldPath<RecorderFormValues>]: FieldPathValue<
    RecorderFormValues,
    K
  > extends T
    ? K
    : never
}[FieldPath<RecorderFormValues>]

type BaseFieldProps<T> = {
  control: Control<RecorderFormValues>
  name: Paths<T>
  label: string
  description?: string
  disabled?: boolean
}

export function RecorderTextField(
  props: BaseFieldProps<string> & { placeholder?: string; readOnly?: boolean }
) {
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <Input
              {...field}
              value={String(field.value ?? '')}
              placeholder={props.placeholder}
              readOnly={props.readOnly}
              disabled={props.disabled}
            />
          </FormControl>
          {props.description && (
            <FormDescription>{props.description}</FormDescription>
          )}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function RecorderNumberField(
  props: BaseFieldProps<number> & { min?: number; max?: number }
) {
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <Input
              type='number'
              min={props.min}
              max={props.max}
              step={1}
              {...safeNumberFieldProps(field)}
              disabled={props.disabled}
            />
          </FormControl>
          {props.description && (
            <FormDescription>{props.description}</FormDescription>
          )}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function RecorderSwitchField(props: BaseFieldProps<boolean>) {
  return (
    <FormField
      control={props.control}
      name={props.name}
      render={({ field }) => (
        <SettingsSwitchItem>
          <SettingsSwitchContent>
            <FormLabel>{props.label}</FormLabel>
            {props.description && (
              <FormDescription>{props.description}</FormDescription>
            )}
          </SettingsSwitchContent>
          <FormControl>
            <Switch
              checked={Boolean(field.value)}
              onCheckedChange={field.onChange}
              disabled={props.disabled}
            />
          </FormControl>
        </SettingsSwitchItem>
      )}
    />
  )
}

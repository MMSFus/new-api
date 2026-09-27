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
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  PICKER_THEME_PRESETS,
  presetLabelKey,
  type ThemePreset,
} from '@/lib/theme-customization'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const defaultThemeSchema = z.object({
  light: z.string(),
  dark: z.string(),
})

type DefaultThemeFormValues = z.infer<typeof defaultThemeSchema>

type DefaultThemeSectionProps = {
  /** Stored option values; an empty string means the built-in default. */
  defaultValues: DefaultThemeFormValues
}

const PICKER_VALUES = new Set<string>(
  PICKER_THEME_PRESETS.map((preset) => preset.value)
)

// Unset or no-longer-offered values show as the built-in default.
function toPickerValue(value: string): ThemePreset {
  return PICKER_VALUES.has(value) ? (value as ThemePreset) : 'default'
}

export function DefaultThemeSection(props: DefaultThemeSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const normalizedDefaults: DefaultThemeFormValues = {
    light: toPickerValue(props.defaultValues.light),
    dark: toPickerValue(props.defaultValues.dark),
  }

  const form = useForm({
    resolver: zodResolver(defaultThemeSchema),
    defaultValues: normalizedDefaults,
  })

  useResetForm(form, normalizedDefaults)

  const onSubmit = async (data: DefaultThemeFormValues) => {
    for (const scheme of ['light', 'dark'] as const) {
      if (data[scheme] === normalizedDefaults[scheme]) continue
      await updateOption.mutateAsync({
        key: `theme_default.${scheme}`,
        value: data[scheme],
      })
    }
  }

  const presetItems = PICKER_THEME_PRESETS.map((preset) => ({
    value: preset.value,
    label: t(presetLabelKey(preset.value)),
  }))

  const fields = [
    {
      name: 'light',
      label: t('When the system is light'),
    },
    {
      name: 'dark',
      label: t('When the system is dark'),
    },
  ] as const

  return (
    <SettingsSection title={t('Default theme')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          {fields.map((item) => (
            <FormField
              key={item.name}
              control={form.control}
              name={item.name}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{item.label}</FormLabel>
                  <Select
                    items={presetItems}
                    onValueChange={field.onChange}
                    value={field.value}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full sm:w-64'>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {presetItems.map((preset) => (
                          <SelectItem key={preset.value} value={preset.value}>
                            {preset.label}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t(
                      'Color theme for visitors who have not picked one. Their own choice always takes priority.'
                    )}
                  </FormDescription>
                </FormItem>
              )}
            />
          ))}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

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
import { Languages, Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useInterfaceLanguage } from '@/hooks/use-interface-language'
import { INTERFACE_LANGUAGE_OPTIONS } from '@/i18n/languages'
import { cn } from '@/lib/utils'

export function LanguageSwitcher() {
  const { t } = useTranslation()
  const language = useInterfaceLanguage()

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={<Button variant='ghost' size='icon' className='h-9 w-9' />}
      >
        <Languages className='size-[1.2rem]' />
        <span className='sr-only'>{t('Change language')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end'>
        <LanguageOptions
          currentLanguage={language.currentLanguage}
          onSelect={language.changeLanguage}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** Language picker nested inside another dropdown menu. */
export function LanguageSubmenu() {
  const { t } = useTranslation()
  const language = useInterfaceLanguage()
  const current = INTERFACE_LANGUAGE_OPTIONS.find(
    (lang) => lang.code === language.currentLanguage
  )

  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger>
        <Languages className='size-4' aria-hidden='true' />
        {t('Change language')}
        <span className='text-muted-foreground ms-auto text-xs'>
          {current?.label}
        </span>
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent>
        <LanguageOptions
          currentLanguage={language.currentLanguage}
          onSelect={language.changeLanguage}
        />
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  )
}

function LanguageOptions(props: {
  currentLanguage: string
  onSelect: (code: string) => void
}) {
  return INTERFACE_LANGUAGE_OPTIONS.map((lang) => (
    <DropdownMenuItem key={lang.code} onClick={() => props.onSelect(lang.code)}>
      {lang.label}
      <Check
        size={14}
        className={cn(
          'ms-auto',
          props.currentLanguage !== lang.code && 'hidden'
        )}
      />
    </DropdownMenuItem>
  ))
}

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
import { Radio as RadioPrimitive } from '@base-ui/react/radio'
import { RadioGroup as Radio } from '@base-ui/react/radio-group'
import { CircleCheck, Palette, RotateCcw } from 'lucide-react'
import type { SVGProps } from 'react'
import { useTranslation } from 'react-i18next'

import { IconThemeDark } from '@/assets/custom/icon-theme-dark'
import { IconThemeLight } from '@/assets/custom/icon-theme-light'
import { IconThemeSystem } from '@/assets/custom/icon-theme-system'
import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { useDirection } from '@/context/direction-provider'
import { useLayout } from '@/context/layout-provider'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useTheme } from '@/context/theme-provider'
import {
  PICKER_THEME_PRESETS,
  presetLabelKey,
  type ThemePreset,
} from '@/lib/theme-customization'
import { cn } from '@/lib/utils'

import { useSidebar } from './ui/sidebar'

const Item = RadioPrimitive.Root

type ConfigDrawerProps = {
  /** Controlled open state; omit to let the drawer manage itself. */
  open?: boolean
  onOpenChange?: (open: boolean) => void
  /**
   * Render the palette icon button that opens the drawer.
   * @default true
   */
  showTrigger?: boolean
}

export function ConfigDrawer(props: ConfigDrawerProps) {
  const { t } = useTranslation()
  const { setOpen } = useSidebar()
  const { resetDir } = useDirection()
  const { resetTheme } = useTheme()
  const { resetLayout } = useLayout()
  const { resetCustomization } = useThemeCustomization()

  const handleReset = () => {
    setOpen(true)
    resetDir()
    resetTheme()
    resetLayout()
    resetCustomization()
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      {props.showTrigger !== false && (
        <SheetTrigger
          render={
            <Button
              size='icon'
              variant='ghost'
              aria-label={t('Open theme settings')}
              aria-describedby='config-drawer-description'
              className='max-md:hidden'
            />
          }
        >
          <Palette className='size-[1.2rem]' aria-hidden='true' />
        </SheetTrigger>
      )}
      <SheetContent className={sideDrawerContentClassName('sm:max-w-md')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Theme Settings')}</SheetTitle>
          <SheetDescription id='config-drawer-description'>
            {t('Choose a color theme and light or dark mode.')}
          </SheetDescription>
        </SheetHeader>
        <div className={sideDrawerFormClassName()}>
          <ThemeConfig />
          <PresetConfig />
        </div>
        <SheetFooter className={sideDrawerFooterClassName('grid-cols-1')}>
          <Button
            variant='destructive'
            onClick={handleReset}
            aria-label={t('Reset all settings to default values')}
          >
            {t('Reset')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function SectionTitle(props: {
  title: string
  showReset?: boolean
  onReset?: () => void
  className?: string
}) {
  return (
    <div
      className={cn(
        'text-muted-foreground mb-2 flex items-center gap-2 text-sm font-semibold',
        props.className
      )}
    >
      {props.title}
      {props.showReset && props.onReset && (
        <Button
          size='icon'
          variant='secondary'
          className='size-4'
          onClick={props.onReset}
          aria-label='Reset'
        >
          <RotateCcw className='size-3' aria-hidden='true' />
        </Button>
      )}
    </div>
  )
}

function RadioGroupItem(props: {
  item: {
    value: string
    label: string
    icon: (props: SVGProps<SVGSVGElement>) => React.ReactElement
  }
  isTheme?: boolean
}) {
  const isTheme = props.isTheme ?? false
  return (
    <Item
      value={props.item.value}
      className={cn('group outline-none', 'transition duration-200 ease-in')}
      aria-label={`Select ${props.item.label.toLowerCase()}`}
      aria-describedby={`${props.item.value}-description`}
    >
      <div
        className={cn(
          'ring-border relative rounded-md ring-[1px]',
          'group-data-checked:ring-primary group-data-checked:shadow-2xl',
          'group-focus-visible:ring-2'
        )}
        role='img'
        aria-hidden='false'
        aria-label={`${props.item.label} option preview`}
      >
        <CircleCheck
          className={cn(
            'fill-primary size-6 stroke-white',
            'group-data-unchecked:hidden',
            'absolute top-0 right-0 translate-x-1/2 -translate-y-1/2'
          )}
          aria-hidden='true'
        />
        <props.item.icon
          className={cn(
            !isTheme &&
              'stroke-primary fill-primary group-data-unchecked:stroke-muted-foreground group-data-unchecked:fill-muted-foreground'
          )}
          aria-hidden='true'
        />
      </div>
      <div
        className='mt-1 text-xs'
        id={`${props.item.value}-description`}
        aria-live='polite'
      >
        {props.item.label}
      </div>
    </Item>
  )
}

function ThemeConfig() {
  const { t } = useTranslation()
  const { defaultTheme, theme, setTheme } = useTheme()
  return (
    <div>
      <SectionTitle
        title={t('Theme')}
        showReset={theme !== defaultTheme}
        onReset={() => setTheme(defaultTheme)}
      />
      <Radio
        value={theme}
        onValueChange={setTheme}
        className='grid w-full max-w-md grid-cols-3 gap-4'
        aria-label={t('Select theme preference')}
        aria-describedby='theme-description'
      >
        {[
          { value: 'system', label: t('System'), icon: IconThemeSystem },
          { value: 'light', label: t('Light'), icon: IconThemeLight },
          { value: 'dark', label: t('Dark'), icon: IconThemeDark },
        ].map((item) => (
          <RadioGroupItem key={item.value} item={item} isTheme />
        ))}
      </Radio>
      <div id='theme-description' className='sr-only'>
        {t('Choose between system preference, light mode, or dark mode')}
      </div>
    </div>
  )
}

function PresetConfig() {
  const { t } = useTranslation()
  const { defaults, customization, setPreset, resetPreset } =
    useThemeCustomization()
  return (
    <div>
      <SectionTitle
        title={t('Color preset')}
        showReset={customization.preset !== defaults.preset}
        onReset={resetPreset}
      />
      <Radio
        value={customization.preset}
        onValueChange={(v) => setPreset(v as ThemePreset)}
        className='grid w-full grid-cols-3 gap-3'
        aria-label={t('Select color preset')}
      >
        {PICKER_THEME_PRESETS.map((preset) => (
          <Item
            key={preset.value}
            value={preset.value}
            className='group flex flex-col items-stretch outline-none'
            aria-label={t(presetLabelKey(preset.value))}
          >
            <div
              className={cn(
                'ring-border relative h-12 rounded-md ring-[1px] transition',
                'group-data-checked:ring-primary group-data-checked:shadow-md',
                'group-focus-visible:ring-2',
                'group-hover:ring-primary/60'
              )}
            >
              <div
                aria-hidden='true'
                className='absolute inset-0 rounded-md'
                style={{
                  background: `linear-gradient(135deg, ${preset.swatches[0]} 50%, ${preset.swatches[1]} 50%)`,
                }}
              />
              <CircleCheck
                className={cn(
                  'fill-primary absolute top-0 right-0 z-10 size-5 translate-x-1/2 -translate-y-1/2 stroke-white',
                  'group-data-unchecked:hidden'
                )}
                aria-hidden='true'
              />
            </div>
            <div className='mt-1.5 truncate text-center text-xs'>
              {t(presetLabelKey(preset.value))}
            </div>
          </Item>
        ))}
      </Radio>
    </div>
  )
}

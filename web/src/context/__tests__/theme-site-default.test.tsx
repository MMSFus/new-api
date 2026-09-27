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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import {
  ThemeCustomizationProvider,
  useThemeCustomization,
} from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { useSystemConfigStore } from '@/stores/system-config-store'

const PRESET_KEY = 'newapi:theme:v1:preset'

function PresetControls() {
  const customization = useThemeCustomization()

  return (
    <>
      <output aria-label='Active preset'>
        {customization.customization.preset}
      </output>
      <button type='button' onClick={() => customization.setPreset('default')}>
        Pick default
      </button>
      <button type='button' onClick={customization.resetPreset}>
        Reset preset
      </button>
    </>
  )
}

function renderWithSiteDefaults(light: string, dark: string) {
  useSystemConfigStore.setState((state) => ({
    config: {
      ...state.config,
      themeDefaultLight: light,
      themeDefaultDark: dark,
    },
  }))
  return render(
    <ThemeProvider>
      <ThemeCustomizationProvider>
        <PresetControls />
      </ThemeCustomizationProvider>
    </ThemeProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  cleanup()
  localStorage.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState())
  document.documentElement.classList.remove('light', 'dark')
  for (const name of document.body.getAttributeNames()) {
    if (name.startsWith('data-theme-')) document.body.removeAttribute(name)
  }
})

describe('site default theme preset', () => {
  it('applies the light site default when the visitor has no preset', () => {
    renderWithSiteDefaults('ocean-breeze', 'rose-garden')

    expect(screen.getByLabelText('Active preset')).toHaveTextContent(
      'ocean-breeze'
    )
    expect(document.body).toHaveAttribute('data-theme-preset', 'ocean-breeze')
  })

  it('applies the dark site default when the resolved scheme is dark', () => {
    localStorage.setItem('newapi:theme:v1:mode', 'dark')

    renderWithSiteDefaults('ocean-breeze', 'rose-garden')

    expect(document.body).toHaveAttribute('data-theme-preset', 'rose-garden')
  })

  it('falls back to the built-in default when the site value is unknown', () => {
    renderWithSiteDefaults('no-such-preset', '')

    expect(screen.getByLabelText('Active preset')).toHaveTextContent('default')
    expect(document.body).not.toHaveAttribute('data-theme-preset')
  })

  it('keeps an explicit visitor choice of the built-in default over the site default', async () => {
    renderWithSiteDefaults('ocean-breeze', 'rose-garden')

    await userEvent.click(screen.getByRole('button', { name: 'Pick default' }))

    expect(localStorage.getItem(PRESET_KEY)).toBe('default')
    expect(document.body).not.toHaveAttribute('data-theme-preset')
  })

  it('returns to the site default after the visitor resets the preset', async () => {
    localStorage.setItem(PRESET_KEY, 'default')
    renderWithSiteDefaults('ocean-breeze', 'rose-garden')

    await userEvent.click(screen.getByRole('button', { name: 'Reset preset' }))

    expect(localStorage.getItem(PRESET_KEY)).toBeNull()
    expect(document.body).toHaveAttribute('data-theme-preset', 'ocean-breeze')
  })
})

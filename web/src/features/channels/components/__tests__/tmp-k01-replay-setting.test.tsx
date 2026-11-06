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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, test } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  buildSettingJSON,
  transformChannelToFormDefaults,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { channelSchema } from '../../types'
import { TmpK01ReplaySetting } from '../tmp-k01-replay-setting'

function SettingsForm(props: {
  channelType: number
  enabled?: boolean
  disabled?: boolean
  onSave: (setting: string) => void
}) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: props.channelType,
      tmp_20261106_k01_enabled: props.enabled,
    },
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) =>
          props.onSave(buildSettingJSON(values))
        )}
      >
        <TmpK01ReplaySetting
          channelType={props.channelType}
          disabled={props.disabled}
        />
        <button type='submit'>Save</button>
      </form>
    </Form>
  )
}

describe('tmp-20261106-k01 channel setting', () => {
  test('Anthropic channels expose a switch and persist its state', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(
      <SettingsForm
        channelType={14}
        onSave={(setting) => saved.push(setting)}
      />
    )
    const toggle = screen.getByRole('switch', {
      name: 'Request replay (tmp-20261106-k01)',
    })
    expect(toggle).not.toBeChecked()
    await user.click(toggle)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(JSON.parse(saved[0]).tmp_20261106_k01_enabled).toBe(true)

    const channel = channelSchema.parse({
      id: 1,
      name: 'K01 channel',
      key: '',
      type: 14,
      status: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      setting: saved[0],
    })
    expect(
      transformChannelToFormDefaults(channel).tmp_20261106_k01_enabled
    ).toBe(true)
  })

  test('other channel types do not expose or save the replay switch', async () => {
    const user = userEvent.setup()
    const saved: string[] = []
    render(
      <SettingsForm
        channelType={1}
        enabled
        onSave={(setting) => saved.push(setting)}
      />
    )
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(JSON.parse(saved[0]).tmp_20261106_k01_enabled).toBeUndefined()
  })

  test('locked channel settings prevent changing the replay switch', async () => {
    const user = userEvent.setup()
    render(<SettingsForm channelType={14} disabled onSave={() => undefined} />)
    const toggle = screen.getByRole('switch', {
      name: 'Request replay (tmp-20261106-k01)',
    })
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    await user.click(toggle)
    expect(toggle).not.toBeChecked()
  })
})

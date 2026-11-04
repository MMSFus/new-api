import { zodResolver } from '@hookform/resolvers/zod'
/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { describe, expect, test, vi } from 'vitest'

import { Form } from '@/components/ui/form'

import { FilterRulesEditor } from '../filter-rules-editor'
import {
  recorderSettingsSchema,
  type RecorderFormValues,
} from '../lib/settings-schema'

vi.mock('@/features/system-settings/operations/session-recorder/api', () => ({
  useRecorderChannels: () => ({
    data: [{ id: 7, name: 'OpenAI', models: [] }],
  }),
  useRecorderGroups: () => ({ data: [] }),
  useRecorderModels: () => ({ data: [] }),
}))

function Fixture() {
  const [saved, setSaved] = useState(false)
  const form = useForm<RecorderFormValues>({
    resolver: zodResolver(recorderSettingsSchema),
    defaultValues: {
      enabled: true,
      producer_enabled: true,
      consumer_enabled: true,
      spool_root: '',
      root_id: '',
      queue_size: 1,
      max_body_bytes: 0,
      record_request_body: true,
      record_response_body: true,
      stats_path: '',
      upload_enabled: false,
      local_cleanup_enabled: false,
      installation_id: '',
      site_id: '',
      host_id: '',
      work_dir: '',
      r2_endpoint: '',
      r2_region: '',
      r2_bucket: '',
      r2_object_prefix: '',
      credentials_dir: '',
      scan_interval_seconds: 5,
      stability_window_seconds: 1,
      max_batch_files: 1,
      max_batch_input_bytes: 1,
      target_archive_bytes: 1,
      gzip_level: 1,
      multipart_concurrency: 1,
      max_upload_mbps: 0,
      adaptive_upload: false,
      max_retries: 0,
      retry_initial_backoff_seconds: 1,
      retry_max_backoff_seconds: 1,
      busy_threshold_percent: 1,
      freshness_seconds: 1,
      external_stats_path: '',
      filter: {
        include: [{ channel_id: 7, channel_search: '', group: '', model: '' }],
        exclude: [],
      },
    },
  })

  return (
    <Form {...form}>
      <>
        <FilterRulesEditor
          form={form}
          name='filter.include'
          title='Include'
          description='Included rules'
        />
        <output>{form.watch('filter.include.0.channel_id')}</output>
        <button type='button' onClick={form.handleSubmit(() => setSaved(true))}>
          Save rules
        </button>
        {saved && <output>Saved</output>}
      </>
    </Form>
  )
}

describe('recorder channel rule input', () => {
  test('search text that is blurred without a selection keeps the saved channel', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    const channel = screen.getByRole('combobox', { name: 'Channel' })

    await user.click(channel)
    await user.clear(channel)
    await user.type(channel, 'OpenAI')
    await user.tab()

    expect(screen.getByText('7', { selector: 'output' })).toBeVisible()
    expect(channel).toHaveValue('OpenAI')
  })

  test('an explicitly submitted positive integer becomes the channel filter', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    const channel = screen.getByRole('combobox', { name: 'Channel' })

    await user.click(channel)
    await user.clear(channel)
    await user.type(channel, '12345')
    await user.keyboard('{Enter}')

    expect(screen.getByText('12345', { selector: 'output' })).toBeVisible()
  })

  test('an unsubmitted channel search shows a validation error', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    const channel = screen.getByRole('combobox', { name: 'Channel' })

    await user.click(channel)
    await user.clear(channel)
    await user.type(channel, 'OpenAI')
    await user.click(screen.getByRole('button', { name: 'Save rules' }))

    expect(
      await screen.findByText('Choose a channel or submit its ID')
    ).toBeVisible()
    expect(channel).toHaveAttribute('aria-invalid', 'true')
    expect(screen.queryByText('Saved')).toBeNull()
    expect(screen.getByText('7', { selector: 'output' })).toBeVisible()
  })

  test('choosing Any channel is the explicit way to remove the channel filter', async () => {
    const user = userEvent.setup()
    render(<Fixture />)
    await user.click(screen.getByRole('combobox', { name: 'Channel' }))
    await user.click(await screen.findByRole('option', { name: 'Any channel' }))

    expect(screen.getByText('0', { selector: 'output' })).toBeVisible()
  })
})

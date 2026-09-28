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
import { useEffect } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Form } from '@/components/ui/form'
import { Separator } from '@/components/ui/separator'

import { SettingsForm } from '../../components/settings-form-layout'
import { SettingsPageFormActions } from '../../components/settings-page-context'
import { SettingsSection } from '../../components/settings-section'
import {
  useSessionRecorderSettings,
  useUpdateSessionRecorderSettings,
} from './api'
import { FilterRulesEditor } from './filter-rules-editor'
import {
  RecorderNumberField,
  RecorderSwitchField,
  RecorderTextField,
} from './form-fields'
import {
  recorderSettingsSchema,
  toApiSettings,
  toFormValues,
  type RecorderFormValues,
} from './lib/settings-schema'
import { SessionRecorderStatusPanel } from './status-panel'
import type { SessionRecorderSettings } from './types'

const GRID = 'grid grid-cols-1 gap-4 md:grid-cols-3'

function SubHeading(props: { title: string; description?: string }) {
  return (
    <div>
      <h4 className='font-medium'>{props.title}</h4>
      {props.description && (
        <p className='text-muted-foreground mt-1 text-xs'>
          {props.description}
        </p>
      )}
    </div>
  )
}

function SessionRecorderForm(props: { settings: SessionRecorderSettings }) {
  const { t } = useTranslation()
  const update = useUpdateSessionRecorderSettings()
  const form = useForm<RecorderFormValues>({
    resolver: zodResolver(recorderSettingsSchema),
    defaultValues: toFormValues(props.settings),
  })
  const control = form.control

  useEffect(() => {
    form.reset(toFormValues(props.settings))
  }, [props.settings, form])

  const enabled = useWatch({ control, name: 'enabled' })
  const producerEnabled = useWatch({ control, name: 'producer_enabled' })
  const consumerEnabled = useWatch({ control, name: 'consumer_enabled' })
  const uploadEnabled = useWatch({ control, name: 'upload_enabled' })
  const producerOff = !enabled || !producerEnabled
  const consumerOff = !enabled || !consumerEnabled
  const uploadOff = consumerOff || !uploadEnabled

  const onSubmit = (values: RecorderFormValues) => {
    update.mutate(toApiSettings(values))
  }

  return (
    <Form {...form}>
      <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
        <SettingsPageFormActions
          onSave={form.handleSubmit(onSubmit)}
          isSaving={update.isPending}
        />
        <SessionRecorderStatusPanel enabled={enabled} />

        <SubHeading
          title={t('Switches')}
          description={t(
            'Recording is off by default. It runs asynchronously and never blocks or fails relay requests; when the queue or disk is full, records are dropped.'
          )}
        />
        <div className={GRID}>
          <RecorderSwitchField
            control={control}
            name='enabled'
            label={t('Enable session recorder')}
          />
          <RecorderSwitchField
            control={control}
            name='producer_enabled'
            label={t('Record relay traffic')}
            description={t('Write matching requests into the local spool')}
            disabled={!enabled}
          />
          <RecorderSwitchField
            control={control}
            name='consumer_enabled'
            label={t('Built-in uploader')}
            description={t(
              'Leave off to only write the spool for an external session-recorder'
            )}
            disabled={!enabled}
          />
        </div>

        <Separator />
        <SubHeading
          title={t('Recording scope')}
          description={t(
            'Record everything when there are no include rules. Exclude rules always win. Model patterns support exact names, prefix* and wildcards.'
          )}
        />
        <FilterRulesEditor
          form={form}
          name='filter.include'
          title={t('Include rules')}
          description={t(
            'Record requests matching any rule (channel / channel + model / group / group + model)'
          )}
          disabled={!enabled}
        />
        <FilterRulesEditor
          form={form}
          name='filter.exclude'
          title={t('Exclude rules')}
          description={t('Never record requests matching any of these rules')}
          disabled={!enabled}
        />

        <Separator />
        <SubHeading title={t('Spool')} />
        <div className={GRID}>
          <RecorderTextField
            control={control}
            name='spool_root'
            label={t('Spool directory')}
            description={t('Absolute path, use a persistent volume')}
            disabled={!enabled}
          />
          <RecorderTextField
            control={control}
            name='root_id'
            label={t('Root ID')}
            disabled={!enabled}
          />
          <RecorderTextField
            control={control}
            name='stats_path'
            label={t('Queue stats file')}
            description={t('Empty: next to the spool directory')}
            disabled={producerOff}
          />
          <RecorderNumberField
            control={control}
            name='queue_size'
            label={t('Queue size')}
            min={1}
            disabled={producerOff}
          />
          <RecorderNumberField
            control={control}
            name='max_body_bytes'
            label={t('Max body bytes')}
            min={0}
            disabled={producerOff}
          />
          <RecorderSwitchField
            control={control}
            name='record_request_body'
            label={t('Record request body')}
            disabled={producerOff}
          />
          <RecorderSwitchField
            control={control}
            name='record_response_body'
            label={t('Record response body')}
            disabled={producerOff}
          />
        </div>

        <Separator />
        <SubHeading
          title={t('Uploader')}
          description={t(
            'Object keys and archive format are identical to the standalone session-recorder. Credentials are read only from AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY or the credentials directory and are never stored.'
          )}
        />
        <div className={GRID}>
          <RecorderTextField
            control={control}
            name='site_id'
            label={t('Site ID')}
            disabled={consumerOff}
          />
          <RecorderTextField
            control={control}
            name='host_id'
            label={t('Host ID')}
            disabled={consumerOff}
          />
          <RecorderTextField
            control={control}
            name='installation_id'
            label={t('Installation ID')}
            readOnly
            description={t('Generated automatically')}
          />
          <RecorderTextField
            control={control}
            name='work_dir'
            label={t('Work directory')}
            description={t(
              'Absolute path outside the spool, persistent volume'
            )}
            disabled={consumerOff}
          />
          <RecorderSwitchField
            control={control}
            name='upload_enabled'
            label={t('Upload to R2')}
            disabled={consumerOff}
          />
          <RecorderSwitchField
            control={control}
            name='local_cleanup_enabled'
            label={t('Delete local files after verified upload')}
            disabled={consumerOff}
          />
          <RecorderTextField
            control={control}
            name='r2_endpoint'
            label={t('R2 endpoint')}
            placeholder='https://<account>.r2.cloudflarestorage.com'
            disabled={uploadOff}
          />
          <RecorderTextField
            control={control}
            name='r2_bucket'
            label={t('R2 bucket')}
            disabled={uploadOff}
          />
          <RecorderTextField
            control={control}
            name='r2_region'
            label={t('R2 region')}
            disabled={uploadOff}
          />
          <RecorderTextField
            control={control}
            name='r2_object_prefix'
            label={t('Object prefix')}
            disabled={uploadOff}
          />
          <RecorderTextField
            control={control}
            name='credentials_dir'
            label={t('Credentials directory')}
            description={t(
              'Empty: $CREDENTIALS_DIRECTORY, then environment variables'
            )}
            disabled={uploadOff}
          />
          <RecorderTextField
            control={control}
            name='external_stats_path'
            label={t('External queue stats file')}
            description={t('Empty: use the built-in producer queue')}
            disabled={consumerOff}
          />
        </div>

        <Separator />
        <SubHeading
          title={t('Resource limits')}
          description={t(
            'Uploads pause while the producer queue is busy or disk / inode usage reaches 90%.'
          )}
        />
        <div className={GRID}>
          <RecorderNumberField
            control={control}
            name='scan_interval_seconds'
            label={t('Scan interval (s)')}
            min={5}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='stability_window_seconds'
            label={t('Stability window (s)')}
            min={1}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='gzip_level'
            label={t('Gzip level')}
            min={1}
            max={9}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='max_batch_files'
            label={t('Max files per batch')}
            min={1}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='max_batch_input_bytes'
            label={t('Max batch input bytes')}
            min={1}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='target_archive_bytes'
            label={t('Target archive bytes')}
            min={1}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='max_upload_mbps'
            label={t('Max upload bandwidth (Mbps)')}
            description={t('0 means unlimited')}
            min={0}
            disabled={uploadOff}
          />
          <RecorderNumberField
            control={control}
            name='multipart_concurrency'
            label={t('Multipart concurrency')}
            min={1}
            disabled={uploadOff}
          />
          <RecorderSwitchField
            control={control}
            name='adaptive_upload'
            label={t('Adaptive upload concurrency')}
            disabled={uploadOff}
          />
          <RecorderNumberField
            control={control}
            name='busy_threshold_percent'
            label={t('Queue busy threshold (%)')}
            min={1}
            max={100}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='freshness_seconds'
            label={t('Queue stats freshness (s)')}
            min={1}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='max_retries'
            label={t('Max retries')}
            min={0}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='retry_initial_backoff_seconds'
            label={t('Initial retry backoff (s)')}
            min={1}
            disabled={consumerOff}
          />
          <RecorderNumberField
            control={control}
            name='retry_max_backoff_seconds'
            label={t('Max retry backoff (s)')}
            min={1}
            disabled={consumerOff}
          />
        </div>
      </SettingsForm>
    </Form>
  )
}

export function SessionRecorderSection() {
  const { t } = useTranslation()
  const settingsQuery = useSessionRecorderSettings()

  return (
    <SettingsSection title={t('Session Recorder')}>
      {settingsQuery.data ? (
        <SessionRecorderForm settings={settingsQuery.data} />
      ) : (
        <p className='text-muted-foreground text-sm'>
          {settingsQuery.isError
            ? t('Failed to load session recorder settings')
            : t('Loading...')}
        </p>
      )}
    </SettingsSection>
  )
}

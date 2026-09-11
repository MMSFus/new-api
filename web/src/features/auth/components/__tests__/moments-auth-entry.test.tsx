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
import { expect, it, vi } from 'vitest'

import { MomentsAuthEntry } from '../moments-auth-entry'

it('presents the configured provider as the recommended sign-in and registration path', async () => {
  const onContinue = vi.fn()
  const user = userEvent.setup()
  render(
    <MomentsAuthEntry
      variant='sign-in'
      displayName='Moments Auth'
      onContinue={onContinue}
    />
  )

  expect(screen.getByText('Recommended')).toBeVisible()
  expect(
    screen.getByText(
      'New users and Moments Auth users should choose this option. Your account will be created automatically the first time you use it.'
    )
  ).toBeVisible()

  await user.click(
    screen.getByRole('button', {
      name: 'Sign in or register with Moments Auth',
    })
  )
  expect(onContinue).toHaveBeenCalledOnce()
})

it('explains automatic account creation on the registration variant', () => {
  render(
    <MomentsAuthEntry
      variant='sign-up'
      displayName='Moments Auth'
      onContinue={() => undefined}
    />
  )

  expect(
    screen.getByText(
      'Your New API account will be created automatically after you continue. You do not need to set another username or password here.'
    )
  ).toBeVisible()
  expect(
    screen.getByRole('button', {
      name: 'Register or sign in with Moments Auth',
    })
  ).toBeEnabled()
})

it('falls back to OIDC and exposes the loading state accessibly', () => {
  render(
    <MomentsAuthEntry
      variant='sign-in'
      displayName='   '
      onContinue={() => undefined}
      loading
    />
  )

  expect(
    screen.getByRole('button', { name: 'Connecting to OIDC...' })
  ).toBeDisabled()
})

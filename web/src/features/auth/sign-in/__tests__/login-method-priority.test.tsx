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
import { beforeEach, expect, it, vi } from 'vitest'

import type { SystemStatus } from '@/features/auth/types'

import { UserAuthForm } from '../components/user-auth-form'

const mocks = vi.hoisted(() => ({
  status: null as SystemStatus | null,
  handleOIDCLogin: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children: React.ReactNode }) => <a>{props.children}</a>,
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: mocks.status, loading: false, error: null }),
}))

vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({ handleLoginResult: vi.fn() }),
}))

vi.mock('@/features/auth/hooks/use-turnstile', () => ({
  useTurnstile: () => ({
    isTurnstileEnabled: false,
    turnstileSiteKey: '',
    turnstileToken: '',
    setTurnstileToken: vi.fn(),
    validateTurnstile: () => true,
  }),
}))

vi.mock('@/features/auth/hooks/use-oauth-login', () => ({
  useOAuthLogin: () => ({
    isLoading: false,
    githubButtonText: 'Continue with GitHub',
    githubButtonDisabled: false,
    handleGitHubLogin: vi.fn(),
    handleDiscordLogin: vi.fn(),
    handleOIDCLogin: mocks.handleOIDCLogin,
    handleLinuxDOLogin: vi.fn(),
    handleTelegramLogin: vi.fn(),
    handleCustomOAuthLogin: vi.fn(),
  }),
}))

vi.mock('@/lib/passkey', () => ({
  isPasskeySupported: () => Promise.resolve(false),
  buildAssertionResult: vi.fn(),
  prepareCredentialRequestOptions: vi.fn(),
}))

beforeEach(() => {
  mocks.handleOIDCLogin.mockReset()
  mocks.status = {
    oidc_enabled: true,
    oidc_display_name: 'Moments Auth',
    oidc_client_id: 'client-id',
    oidc_authorization_endpoint: 'https://auth.example/authorize',
    password_login_enabled: true,
    passkey_login: false,
  }
})

it('makes Moments Auth the single recommended identity entry', async () => {
  const user = userEvent.setup()
  render(<UserAuthForm />)

  expect(screen.getByText('Recommended')).toBeVisible()
  const identityButton = screen.getByRole('button', {
    name: 'Sign in or register with Moments Auth',
  })
  expect(
    screen.queryByRole('button', { name: 'Continue with Moments Auth' })
  ).not.toBeInTheDocument()

  await user.click(identityButton)
  expect(mocks.handleOIDCLogin).toHaveBeenCalledOnce()
})

it('labels password authentication as an on-site account option', () => {
  render(<UserAuthForm />)

  expect(screen.getByText('On-site account sign-in')).toBeVisible()
  expect(
    screen.getByText(
      'Only use this option if you have already set a username and password for this site.'
    )
  ).toBeVisible()
  expect(screen.getByLabelText('Username or Email')).toBeVisible()
  expect(screen.getByLabelText('Password')).toBeVisible()
})

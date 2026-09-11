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

import { SignUpForm } from '../components/sign-up-form'

const mocks = vi.hoisted(() => ({
  status: null as SystemStatus | null,
  loading: false,
  handleOIDCLogin: vi.fn(),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: mocks.status,
    loading: mocks.loading,
    error: null,
  }),
}))

vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({
    redirectToLogin: vi.fn(),
    handleLoginResult: vi.fn(),
  }),
}))

vi.mock('@/features/auth/hooks/use-email-verification', () => ({
  useEmailVerification: () => ({
    isSending: false,
    secondsLeft: 0,
    isActive: false,
    sendCode: vi.fn(),
  }),
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

beforeEach(() => {
  mocks.loading = false
  mocks.handleOIDCLogin.mockReset()
  mocks.status = {
    register_enabled: true,
    password_register_enabled: false,
    oidc_enabled: true,
    oidc_display_name: 'Moments Auth',
    email_verification: true,
  }
})

it('uses only Moments Auth when password registration is disabled', async () => {
  const user = userEvent.setup()
  render(<SignUpForm />)

  expect(
    screen.getByText(
      'This site uses Moments Auth for registration. Continue to create or access your New API account.'
    )
  ).toBeVisible()
  expect(screen.queryByLabelText('Username')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
  expect(screen.queryByText('Send code')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Create account' })
  ).not.toBeInTheDocument()

  await user.click(
    screen.getByRole('button', {
      name: 'Register or sign in with Moments Auth',
    })
  )
  expect(mocks.handleOIDCLogin).toHaveBeenCalledOnce()
})

it('preserves password registration when it is enabled', () => {
  mocks.status = {
    ...mocks.status,
    password_register_enabled: true,
  }
  render(<SignUpForm />)

  expect(screen.getByLabelText('Username')).toBeVisible()
  expect(screen.getByLabelText('Password')).toBeVisible()
  expect(screen.getByLabelText('Confirm password')).toBeVisible()
  expect(screen.getByText('Send code')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
})

it('shows a closed state when all registration is disabled', () => {
  mocks.status = {
    register_enabled: false,
    password_register_enabled: false,
    oidc_enabled: true,
  }
  render(<SignUpForm />)

  expect(screen.getByText('Registration is closed')).toBeVisible()
  expect(
    screen.getByText(
      'This system is not currently accepting new account registrations.'
    )
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: /Moments Auth/ })
  ).not.toBeInTheDocument()
})

it('shows an unavailable state when no registration method can create an account', () => {
  mocks.status = {
    register_enabled: true,
    password_register_enabled: false,
    oidc_enabled: false,
  }
  render(<SignUpForm />)

  expect(screen.getByText('Registration is unavailable')).toBeVisible()
  expect(
    screen.getByText(
      'There is currently no registration method available. Please contact the administrator.'
    )
  ).toBeVisible()
})

it('does not flash the password form while status is loading', () => {
  mocks.status = null
  mocks.loading = true
  render(<SignUpForm />)

  expect(
    screen.getByRole('status', { name: 'Loading registration options' })
  ).toBeVisible()
  expect(screen.queryByLabelText('Username')).not.toBeInTheDocument()
})

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
import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { HeroTerminalDemo } from '../hero-terminal-demo'

interface StorefrontHeroProps {
  isAuthenticated: boolean
}

/**
 * Home page opener: the site's promise (low price, one unified API) and the
 * next step. Signed-out visitors are sent to sign up or sign in, never to the
 * model list, which stays behind sign-in.
 */
export function StorefrontHero(props: StorefrontHeroProps) {
  const { t } = useTranslation()

  return (
    <section className='px-4 pt-24 pb-12 sm:px-6 md:pt-32 md:pb-16'>
      <div className='mx-auto grid max-w-6xl grid-cols-1 items-center gap-10 lg:grid-cols-2 lg:gap-12'>
        <div className='flex min-w-0 flex-col items-start'>
          <h1 className='text-[clamp(2rem,5vw,3.25rem)] leading-tight font-bold tracking-tight text-balance'>
            {t('Low-cost, unified AI API')}
          </h1>
          <p className='text-muted-foreground mt-5 max-w-xl text-base leading-relaxed'>
            {t(
              'One API key for OpenAI, Claude, Gemini and more. Works with the OpenAI format, and you only pay for what you use.'
            )}
          </p>

          <div className='mt-8 flex flex-wrap items-center gap-3'>
            {props.isAuthenticated ? (
              <>
                <Button
                  className='group h-11 px-5'
                  render={<Link to='/dashboard' />}
                >
                  {t('Go to Dashboard')}
                  <ArrowRight
                    className='size-4 transition-transform group-hover:translate-x-0.5'
                    aria-hidden='true'
                  />
                </Button>
                <Button
                  variant='outline'
                  className='h-11 px-5'
                  render={<Link to='/pricing' />}
                >
                  {t('Model Square')}
                </Button>
              </>
            ) : (
              <>
                <Button
                  className='group h-11 px-5'
                  render={<Link to='/sign-up' />}
                >
                  {t('Get Started')}
                  <ArrowRight
                    className='size-4 transition-transform group-hover:translate-x-0.5'
                    aria-hidden='true'
                  />
                </Button>
                <Button
                  variant='outline'
                  className='h-11 px-5'
                  render={<Link to='/sign-in' />}
                >
                  {t('Sign in')}
                </Button>
              </>
            )}
          </div>
        </div>

        <div className='flex min-w-0 justify-center'>
          <HeroTerminalDemo />
        </div>
      </div>
    </section>
  )
}

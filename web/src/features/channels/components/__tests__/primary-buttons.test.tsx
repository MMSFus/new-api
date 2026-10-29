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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory,createRootRoute,createRoute,createRouter,RouterProvider } from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test } from 'vitest'

import { ChannelsPrimaryButtons } from '../channels-primary-buttons'
import { ChannelsProvider, useChannels } from '../channels-provider'

function ModeState() {
  const channels = useChannels()
  return (
    <output aria-label='modes'>
      {[
        channels.batchMode && 'batch',
        channels.enableTagMode && 'tag',
      ]
        .filter(Boolean)
        .join(',')}
    </output>
  )
}

function renderButtons() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const root=createRootRoute()
  const authenticated=createRoute({getParentRoute:()=>root,id:'_authenticated'})
  const channels=createRoute({getParentRoute:()=>authenticated,path:'/channels/',component:()=> <ChannelsProvider><ChannelsPrimaryButtons/><ModeState/></ChannelsProvider>})
  const router=createRouter({routeTree:root.addChildren([authenticated.addChildren([channels])]),history:createMemoryHistory({initialEntries:['/channels/']})})
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router}/>
    </QueryClientProvider>
  )
  return userEvent.setup()
}

afterEach(() => {
  localStorage.clear()
})

test('display modes are collapsed into one menu instead of inline switches', async () => {
  renderButtons()

  expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  expect(await screen.findByRole('button', { name: 'Display options' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'More actions' })).toBeVisible()
})

test('choosing modes from the display menu updates and remembers them', async () => {
  const user = renderButtons()

  await user.click(await screen.findByRole('button', { name: 'Display options' }))
  for (const name of ['Tag Mode', 'Batch Operations']) {
    const item = await screen.findByRole('menuitemcheckbox', { name })
    await user.click(item)
    expect(item).toHaveAttribute('aria-checked', 'true')
  }

  expect(screen.getByRole('status', { name: 'modes' })).toHaveTextContent(
    'batch,tag'
  )
  expect(localStorage.getItem('enable-tag-mode')).toBe('true')
})

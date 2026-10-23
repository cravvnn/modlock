import { expect, test } from 'bun:test'

import { Face, UiRequest } from '../../src/wasm/ui/ui.pb.js'
import { Call, Reply } from '../../src/wasm/wasm.pb.js'
import { bind } from '../src/host.js'
import { jsx, show } from '../src/index.js'

// The library sends each interface change through the transport, which
// records it.
const sent: UiRequest[] = []
bind((encoded) => {
  const call = Call.fromBinary(encoded)
  if (call.method === 'Ui') sent.push(UiRequest.fromBinary(call.request))
  return Reply.toBinary({})
})

/** keys is a panel of one label per key, the latest of them lit. */
function keys(lit: string) {
  return jsx('panel', {
    children: ['W', 'A', 'S', 'D'].map((key) =>
      jsx(
        'label',
        {
          style: { color: key === lit ? '#ffffff' : '#808080' },
          children: key,
        },
        key,
      ),
    ),
  })
}

test('a redraw sends only the nodes that changed, under their first ids', () => {
  // The first draw sends the whole screen.
  const player = { slot: 7 } as Parameters<typeof show>[0]
  show(player, keys('W'))
  const first = sent.at(-1)!.change!
  expect(first.reset).toBe(true)
  const ids = new Map(first.set!.map((node) => [node.text ?? '', node.id]))

  // Lighting another key changes its label and the one that went dark.
  show(player, keys('A'))
  const second = sent.at(-1)!.change!
  expect(second.reset ?? false).toBe(false)
  expect(second.set!.map((node) => node.text)).toEqual(['W', 'A'])
  expect(second.set!.map((node) => node.id)).toEqual([
    ids.get('W'),
    ids.get('A'),
  ])

  // The same screen again sends nothing.
  const count = sent.length
  show(player, keys('A'))
  expect(sent.length).toBe(count)
})

test("a style names the game's faces and edge lines on the wire", () => {
  // Show a title in the heading face over a bordered plate.
  const player = { slot: 8 } as Parameters<typeof show>[0]
  show(
    player,
    jsx('panel', {
      style: { borderWidth: 2, borderColor: '#ffefd71a' },
      children: jsx('label', {
        style: {
          face: 'block',
          uppercase: true,
          letterSpacing: 1,
          shadow: true,
        },
        children: 'Round 4',
      }),
    }),
  )
  const nodes = sent.at(-1)!.change!.set!
  const plate = nodes.find((node) => node.style?.borderWidth)!
  const title = nodes.find((node) => node.text === 'Round 4')!

  // Each field arrives typed: the face as its enum, the color as RGBA.
  expect(plate.style!.borderColor).toBe(0xffefd71a)
  expect(title.style!.face).toBe(Face.BLOCK)
  expect(title.style!.uppercase).toBe(true)
  expect(title.style!.letterSpacing).toBe(1)
  expect(title.style!.shadow).toBe(true)
})

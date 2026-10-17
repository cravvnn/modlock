import { expect, test } from 'bun:test'

import { Call, Reply, UiRequest } from '../../proto/modlock/wasm.pb.js'

// The library sends each interface change through the bridge, which records
// it. Test files share the library's modules, so a bridge another file
// installs later takes the calls in its place.
interface Bridge {
  hostCall(call: Uint8Array): Uint8Array
}
const global = globalThis as unknown as { __modlock: Bridge }
const sent: UiRequest[] = []
const bridge: Bridge = {
  hostCall: (encoded) => {
    if (global.__modlock !== bridge) return global.__modlock.hostCall(encoded)
    const call = Call.fromBinary(encoded)
    if (call.method === 'Ui') sent.push(UiRequest.fromBinary(call.request))
    return Reply.toBinary({})
  },
}
global.__modlock = bridge
const { jsx, show } = await import('../src/index.js')

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

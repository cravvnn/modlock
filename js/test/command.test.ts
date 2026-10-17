import { expect, test } from 'bun:test'

import {
  Call,
  CommandEvent,
  CommandResult,
  FrameEvent,
  Reply,
} from '../../proto/modlock/wasm.pb.js'

// The library answers every host call it makes with an empty reply. Test
// files share the library's modules, so a bridge another file installs later
// takes the calls in its place.
interface Bridge {
  hostCall(call: Uint8Array): Uint8Array
  event?: (call: Uint8Array) => Uint8Array
}
const global = globalThis as unknown as { __modlock: Bridge }
const bridge: Bridge = {
  hostCall: (call) =>
    global.__modlock === bridge
      ? Reply.toBinary({})
      : global.__modlock.hostCall(call),
}
global.__modlock = bridge
const { command, onFrame } = await import('../src/index.js')

/** deliver sends the mod one host event and returns its decoded reply. */
function deliver(method: string, request: Uint8Array): Reply {
  return Reply.fromBinary(bridge.event!(Call.toBinary({ method, request })))
}

/** frame delivers a frame at seconds on the game clock. */
function frame(seconds: number): void {
  deliver('Frame', FrameEvent.toBinary({ timeSeconds: seconds }))
}

/** send delivers a command line from the player in slot and reports whether the mod claimed it. */
function send(slot: number, line: string): boolean {
  const reply = deliver(
    'Command',
    CommandEvent.toBinary({ player: slot, line }),
  )
  return (
    CommandResult.fromBinary(reply.response ?? new Uint8Array()).claimed ??
    false
  )
}

test('a command is claimed at once and runs with the next frame', () => {
  // Record each command and frame in the order the mod sees them.
  const seen: string[] = []
  command('watch', (player, args) => seen.push(`watch ${player.slot} ${args}`))
  onFrame(() => seen.push('frame'))
  frame(10)
  seen.length = 0

  // The command is claimed but waits for the frame, and runs before its handlers.
  expect(send(3, '  watch  b0fe  ')).toBe(true)
  expect(seen).toEqual([])
  frame(10.016)
  expect(seen).toEqual(['watch 3 b0fe', 'frame'])

  // A command the mod does not know stays with the game.
  expect(send(3, 'unknown')).toBe(false)
})

test('a command that throws does not stop the frame', () => {
  // The failing command logs, and the next one and the frame handlers still run.
  const seen: string[] = []
  command('fail', () => {
    throw new Error('broken')
  })
  command('ok', () => seen.push('ok'))
  frame(20)
  send(1, 'fail')
  send(1, 'ok')
  frame(20.016)
  expect(seen).toEqual(['ok'])
})

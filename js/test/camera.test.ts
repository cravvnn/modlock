import { expect, test } from 'bun:test'

import { CameraOptions } from '../../src/wasm/players/players.pb.js'
import { Call, Reply } from '../../src/wasm/wasm.pb.js'
import { cameraPath, cameraPulse, cameraShake } from '../src/camera.js'
import { Player } from '../src/host.gen.js'
import { bind } from '../src/host.js'

/** fail names the call the host answers with an error. */
let fail = ''

/** sent holds the camera calls the host received. */
const sent: { method: string; options?: CameraOptions }[] = []
bind((bytes) => {
  const { method = '', request = new Uint8Array() } = Call.fromBinary(bytes)
  sent.push({
    method,
    options:
      method === 'MoveCamera' ? CameraOptions.fromBinary(request) : undefined,
  })
  return Reply.toBinary(method === fail ? { error: 'no hero' } : {})
})

/** lerps returns each sent lerp as its start, end, duration and delay. */
function lerps(): number[][] {
  return sent.flatMap(({ options }) => {
    // Every key in these tests is a number.
    const motion = options?.motion?.motion
    if (motion?.case !== 'lerp') return []
    const { start, end, duration } = motion.value
    const scalar = (v: typeof start) =>
      v?.value?.case === 'number' ? v.value.value : NaN
    return [[scalar(start), scalar(end), duration ?? 0, options?.delay ?? 0]]
  })
}

test('camera paths queue one lerp per segment in their own context', () => {
  // Each helper sends to the host bound above.
  const player = new Player(3)

  // A pulse moves out, holds and moves back, relative to the usual value.
  sent.length = 0
  const pulse = cameraPulse(player, {
    param: 'distance',
    by: 300,
    in: 0.5,
    hold: 2,
    out: 1,
  })!
  expect(pulse.duration).toBe(3.5)
  expect(lerps()).toEqual([
    [0, 300, 0.5, 0],
    [300, 300, 2, 0],
    [300, 0, 1, 0],
  ])
  expect(sent.every(({ options }) => options?.relative)).toBe(true)
  expect(new Set(sent.map(({ options }) => options?.context))).toEqual(
    new Set([pulse.context]),
  )

  // A path waits for its first key, skips a jump, and its cancel clears
  // only its own context.
  sent.length = 0
  const path = cameraPath(player, {
    param: 'fieldOfView',
    keys: [
      { at: 1, value: 90 },
      { at: 1, value: 60 },
      { at: 2, value: 70 },
    ],
  })!
  expect(path.context).not.toBe(pulse.context)
  expect(lerps()).toEqual([[60, 70, 1, 1]])
  sent.length = 0
  path.cancel()
  expect(sent.map(({ method }) => method)).toEqual(['ClearCamera'])

  // A shake alternates sides, shrinks and settles at zero.
  sent.length = 0
  cameraShake(player, { amplitude: 20, duration: 0.5, frequency: 8 })
  const swings = lerps().map(([, end]) => end)
  expect(swings).toEqual([20, -15, 10, -5, 0])

  // A path the host refuses is cleared and reports nothing.
  fail = 'MoveCamera'
  sent.length = 0
  expect(
    cameraPulse(player, { param: 'distance', by: 1, in: 1, hold: 1, out: 1 }),
  ).toBeUndefined()
  expect(sent.at(-1)?.method).toBe('ClearCamera')
  fail = ''
})

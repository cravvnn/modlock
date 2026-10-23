// The hideout's orbit camera on any map. The pawn's networked
// m_bInHideoutZone moves its client to the hideout view: the orbit camera at
// the game's entry level, with the hideout HUD in place of the game HUD. The
// wheel zooms only while the pawn also holds MODIFIER_STATE_HIDEOUT_ZONE, since
// the server ignores the wheel's orbit_camera_in and orbit_camera_out without
// it.
//
// The helpers below build camera paths from moveCamera's queued lerps: the
// operations on one param in one context run in order, so a whole path goes
// to the client at once and plays without the mod's timers.

import {
  type CameraParam,
  type CameraValue,
  holdModifierState,
  type Player,
  writeField,
} from './host.gen.js'

/**
 * setOrbitCamera gives the player's hero the hideout's orbit camera, zoomed
 * with the wheel, or returns the normal camera. The hideout HUD replaces the
 * game HUD while it is on. It returns false when the player has no hero or a
 * call fails.
 */
export function setOrbitCamera(player: Player, enabled: boolean): boolean {
  // A player between lives has no hero to switch.
  const pawn = player.pawn()
  if (!pawn?.entity) return false

  // The flag picks the view; the held state lets the wheel zoom it.
  const flag = writeField(
    pawn.entity,
    'CCitadelPlayerPawn',
    'm_bInHideoutZone',
    'bool',
    enabled,
  )
  const zoom = holdModifierState(
    pawn.entity,
    'MODIFIER_STATE_HIDEOUT_ZONE',
    enabled,
  )
  return flag && zoom
}

/** CameraKey is one point of a camera path: a value at a time. */
export interface CameraKey {
  /** at is the time of the key, in seconds after the path starts. */
  at: number
  /** value is the param's value at the key. */
  value: CameraValue
  /** bias and gain shape the curve from the key before this one. */
  bias?: number
  gain?: number
}

/** CameraPathOptions moves one param of a player's camera through keys. */
export interface CameraPathOptions {
  /** param is the camera value the path moves. */
  param: CameraParam
  /**
   * keys are the path's points in time order. The param moves in a line
   * from each to the next, holds where two keys share a value, and jumps
   * where two share a time.
   */
  keys: readonly CameraKey[]
  /** relative adds the keys' values to the param's usual value. */
  relative?: boolean
  /** context names the path's operations; omitted, a name of its own. */
  context?: string
  /** priority decides between this path and another context's moves. */
  priority?: number
  /** target is the entity handle of the hero whose camera moves. */
  target?: number
}

/** CameraMove is a path running on a player's camera. */
export interface CameraMove {
  /** context names the path's operations. */
  readonly context: string
  /** duration is how long the path runs, in seconds. */
  readonly duration: number
  /** cancel ends the path at once and returns the param to its usual value. */
  cancel(): boolean
}

/** paths counts the paths started, to give each its own context. */
let paths = 0

/**
 * cameraPath moves one param of the player's camera through keys, sent at
 * once as a queue of lerps in one context. The param returns to its usual
 * value after the last key, so a path ends where the camera should rest: at
 * zero when relative. Two paths in their own contexts take turns by priority.
 * It returns undefined when the player has no hero or a call fails.
 */
export function cameraPath(
  player: Player,
  options: CameraPathOptions,
): CameraMove | undefined {
  // A path without keys moves nothing.
  const { param, keys, relative, priority, target } = options
  const context = options.context ?? `modlock.camera.${++paths}`
  const [first, ...rest] = keys
  if (!first) return undefined

  // Each segment between two keys is one lerp, queued after the one before.
  // Only the first operation waits for the first key.
  let sent = true
  let delay = first.at > 0 ? first.at : undefined
  let from = first
  for (const to of rest) {
    const duration = to.at - from.at
    if (duration > 0) {
      const lerp = {
        start: from.value,
        end: to.value,
        bias: to.bias,
        gain: to.gain,
        duration,
      }
      sent &&= player.moveCamera({
        context,
        param,
        motion: { lerp },
        delay,
        relative,
        priority,
        target,
      })
      delay = undefined
    }
    from = to
  }

  // A path that half reached the camera is taken back off it.
  if (!sent) {
    player.clearCamera(context)
    return undefined
  }
  return {
    context,
    duration: from.at,
    cancel: () => player.clearCamera(context),
  }
}

/** CameraPulseOptions moves a param away, holds it and brings it back. */
export interface CameraPulseOptions {
  /** param is the camera value the pulse moves. */
  param: CameraParam
  /** by is how far the param moves from its usual value. */
  by: CameraValue
  /** in is how long the move out takes, in seconds. */
  in: number
  /** hold is how long the param stays out, in seconds. */
  hold: number
  /** out is how long the move back takes, in seconds. */
  out: number
  /** context names the pulse's operations; omitted, a name of its own. */
  context?: string
  /** priority decides between this pulse and another context's moves. */
  priority?: number
  /** target is the entity handle of the hero whose camera moves. */
  target?: number
}

/**
 * cameraPulse moves one param of the player's camera by an amount, holds it
 * there and brings it back, as an ability pulls the camera back while it
 * charges. It returns undefined when the player has no hero or a call fails.
 */
export function cameraPulse(
  player: Player,
  options: CameraPulseOptions,
): CameraMove | undefined {
  const { by, in: rise, hold, out } = options
  const rest = zero(by)
  return cameraPath(player, {
    ...options,
    relative: true,
    keys: [
      { at: 0, value: rest },
      { at: rise, value: by },
      { at: rise + hold, value: by },
      { at: rise + hold + out, value: rest },
    ],
  })
}

/** CameraShakeOptions shakes a param around its usual value. */
export interface CameraShakeOptions {
  /** param is the camera value that shakes; omitted, horizontalOffset. */
  param?: CameraParam
  /** amplitude is how far the first swing reaches. */
  amplitude: number
  /** duration is how long the shake runs, in seconds. */
  duration: number
  /** frequency is how many swings each way per second; omitted, 12. */
  frequency?: number
  /** context names the shake's operations; omitted, a name of its own. */
  context?: string
  /** priority decides between this shake and another context's moves. */
  priority?: number
  /** target is the entity handle of the hero whose camera moves. */
  target?: number
}

/**
 * cameraShake swings one param of the player's camera to either side of its
 * usual value, each swing smaller than the one before, and settles back. It
 * returns undefined when the player has no hero or a call fails.
 */
export function cameraShake(
  player: Player,
  options: CameraShakeOptions,
): CameraMove | undefined {
  // Swings alternate sides at even spacing and shrink to zero at the end.
  const { amplitude, duration } = options
  const swings = Math.max(1, Math.round(duration * (options.frequency ?? 12)))
  const keys: CameraKey[] = [{ at: 0, value: 0 }]
  for (let i = 1; i <= swings; i++) {
    const side = i % 2 === 1 ? 1 : -1
    const value = side * amplitude * (1 - (i - 1) / swings)
    keys.push({ at: (duration * i) / (swings + 1), value })
  }
  keys.push({ at: duration, value: 0 })
  return cameraPath(player, {
    ...options,
    param: options.param ?? 'horizontalOffset',
    relative: true,
    keys,
  })
}

/** zero returns the zero of a camera value's kind. */
function zero(value: CameraValue): CameraValue {
  return typeof value === 'number' ? 0 : { x: 0, y: 0, z: 0 }
}

// The hideout's orbit camera on any map. The pawn's networked
// m_bInHideoutZone moves its client to the hideout view: the orbit camera at
// the game's entry level, with the hideout HUD in place of the game HUD. The
// wheel zooms only while the pawn also holds MODIFIER_STATE_HIDEOUT_ZONE, since
// the server ignores the wheel's orbit_camera_in and orbit_camera_out without
// it.

import { holdModifierState, type Player, writeField } from './host.gen.js'

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

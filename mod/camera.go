package mod

import "errors"

// SetOrbitCamera gives the player's hero the hideout's orbit camera, zoomed
// with the wheel, or returns the normal camera. The hideout HUD replaces the
// game HUD while it is on.
//
// The pawn's networked m_bInHideoutZone moves its client to the hideout view,
// and the wheel zooms only while the pawn holds MODIFIER_STATE_HIDEOUT_ZONE,
// since the server ignores orbit_camera_in and orbit_camera_out without it.
func SetOrbitCamera(player Player, enabled bool) error {
	pawn, err := player.Pawn()
	if err != nil {
		return err
	}
	entity := pawn.GetEntity()
	if entity == 0 {
		return errors.New("the player has no hero")
	}

	// The flag picks the view; the held state lets the wheel zoom it.
	return errors.Join(
		WriteField(entity, "CCitadelPlayerPawn", "m_bInHideoutZone", FieldTypeBool, enabled),
		HoldModifierState(entity, "MODIFIER_STATE_HIDEOUT_ZONE", &enabled),
	)
}

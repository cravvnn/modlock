// The host reports the occupied player slots before an event whenever they
// changed, so players answers from memory instead of a call each frame.

import { PlayersResponse } from '../../proto/modlock/wasm.pb.js'
import { type Connection, Player } from './host.gen.js'
import { call } from './host.js'

/** roster is the player list the host last reported, if any. */
let roster: Connection[] | undefined = undefined

/** keepPlayers keeps the player list the host reported. */
export function keepPlayers(players: Connection[]): void {
  roster = players
}

/**
 * players lists the occupied player slots in slot order. It answers from the
 * roster the host last reported, and asks the host until a report arrives.
 */
export function players(): Connection[] {
  if (roster !== undefined) return roster.slice()
  const reply = call('Players', new Uint8Array())
  if (reply === undefined) return []
  return (PlayersResponse.fromBinary(reply).players ?? []).map((c) => ({
    player: new Player(c.player ?? 0),
    steamId: c.steamId ?? 0n,
    name: c.name ?? '',
    bot: c.bot ?? false,
    ready: c.ready ?? false,
    generation: c.generation ?? 0,
  }))
}

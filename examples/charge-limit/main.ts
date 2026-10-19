// Charge limit checks setAbility's maxCharges. Started with the argument
// probe, the mod seats two bots on a hero with a charged ability, raises the
// most charges of one and leaves the other as it is, empties both and keeps
// each recharge short. The raised bot should come back to its new maximum and
// the other to the game's own. The mod quits the server with PASS or FAIL.
import {
  addBot,
  log,
  onFrame,
  onStart,
  serverCommand,
  type Player,
} from 'modlock'

/** hero is the bots' hero, whose Card Toss holds two charges. */
const hero = 'hero_wraith'

/** target is the ability the check empties, and baseCharges its charges in the game's scripts. */
const target = 'citadel_ability_card_toss'
const baseCharges = 2

/** extra is how many charges the raised bot's maximum adds. */
const extra = 2

/** timeout bounds the whole check in seconds. */
const timeout = 40

/** shortest is how soon a charge returns once the check empties an ability. */
const shortest = 0.5

/** Bot is one seated bot and the ability the check watches. */
interface Bot {
  readonly label: string
  readonly player: Player
  readonly raised: boolean
  ability?: string
}

const bots: Bot[] = []
let started: number | undefined
let native = 0
let done = false
let probing = false
let unlocked = false
let logged = -1

onStart((args) => {
  probing = args.includes('probe')
})

/** seat adds the two bots once the server is up. */
function seat(): boolean {
  for (const [index, raised] of [false, true].entries()) {
    const label = raised ? 'raised' : 'control'
    const player = addBot({ name: `Charge ${label}`, team: 2 + index, hero })
    if (player === undefined) {
      finish(`the ${label} bot could not join`)
      return false
    }
    bots.push({ label, player, raised })
  }
  return true
}

onFrame((frame) => {
  // Do nothing unless the probe runs.
  if (done || !probing) {
    return
  }

  // Seat the bots on the first frame, once the server is up.
  if (bots.length === 0 && !seat()) {
    return
  }
  started ??= frame.timeSeconds
  const elapsed = frame.timeSeconds - started
  if (elapsed > timeout) {
    finish('timed out')
    return
  }

  // Wait until every bot has a hero with abilities.
  const abilities = bots.map((bot) => bot.player.abilities())
  if (abilities.some((list) => list.length === 0)) {
    return
  }

  // Unlock the ability keys, which hold no charges while locked, then pick the
  // first that holds several charges and empty it.
  if (native === 0) {
    if (!unlocked) {
      unlocked = true
      for (const bot of bots) {
        for (const ability of bot.player.abilities()) {
          if (ability.slot < 3) {
            bot.player.setAbility({ ability: ability.name, upgrades: 1 })
          }
        }
      }
      return
    }
    if (elapsed < 2) {
      return
    }
    const charged = abilities[0].find((item) => item.name === target)
    if (charged === undefined) {
      finish(`the hero has no ${target}`)
      return
    }
    native = baseCharges
    for (const bot of bots) {
      bot.ability = charged.name
      bot.player.setAbility({
        ability: charged.name,
        ...(bot.raised ? { maxCharges: native + extra } : {}),
        charges: 0,
        rechargeEnd: frame.timeSeconds + shortest,
      })
    }
    log(`${charged.name} holds ${native} charges; raising one bot by ${extra}`)
    return
  }

  // Keep each recharge short until the check ends.
  const held: number[] = []
  for (const [index, bot] of bots.entries()) {
    const ability = abilities[index].find((item) => item.name === bot.ability)
    if (ability === undefined) {
      finish(`the ${bot.label} bot lost its ability`)
      return
    }
    held.push(ability.charges)
    // A recharge starts only after a use, so start one the check arms itself.
    if (
      ability.rechargeEnd === 0 ||
      ability.rechargeEnd > frame.timeSeconds + shortest
    ) {
      bot.player.setAbility({
        ability: ability.name,
        rechargeEnd: frame.timeSeconds + shortest,
      })
    }
  }
  if (Math.floor(elapsed) !== logged) {
    logged = Math.floor(elapsed)
    log(`t=${elapsed.toFixed(1)} control=${held[0]} raised=${held[1]}`)
  }
  if (elapsed > 15) {
    const ok = held[0] === native && held[1] === native + extra
    finish(
      ok ? undefined : `control held ${held[0]}, raised held ${held[1]}`,
      `native ${native}`,
    )
  }
})

/** finish logs the verdict and quits the server. */
function finish(problem?: string, note = ''): void {
  done = true
  log(
    problem === undefined
      ? `Charge limit probe PASS ${note}`
      : `Charge limit probe FAIL: ${problem}`,
  )
  serverCommand('quit')
}

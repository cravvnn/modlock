//go:build deps_only

#include <algorithm>
#include <array>
#include <span>
#include <utility>

#include "modlock/gameinterop/native_memory.h"
#include "wasm/game.h"
#include "wasm/game_internal.h"

namespace modlock::wasm {

std::expected<players::HeroResponse, std::string> Game::SelectHero(const players::SelectHeroRequest& request) {
  // Resolve the hero and the selection service.
  auto heroes = services_.Heroes();
  if (!heroes) {
    return std::unexpected(heroes.error());
  }
  auto hero = FindHero(**heroes, request);
  if (!hero) {
    return std::unexpected(hero.error());
  }
  auto selection = services_.Selection();
  if (!selection) {
    return std::unexpected(selection.error());
  }

  // Select the hero for the player in that slot.
  const auto state = gameinterop::ConnectionTracker::StateForSlot(request.player());
  if (!state.occupied) {
    return std::unexpected("no player is in that slot");
  }
  auto selected =
      observer_.SelectPlayer(request.player(), state.xuid, state.generation, request.team(),
                             hero->native_definition_pointer, **selection);
  if (!selected) {
    return std::unexpected(selected.error());
  }

  // Answer the hero selected.
  players::HeroResponse response;
  response.set_hero(static_cast<uint32_t>(hero->id));
  return response;
}

std::expected<void, std::string> Game::Spectate(const players::PlayerRequest& request) {
  // Resolve the selection service and the player in that slot.
  auto selection = services_.Selection();
  if (!selection) {
    return std::unexpected(selection.error());
  }
  const auto state = gameinterop::ConnectionTracker::StateForSlot(request.player());
  if (!state.occupied) {
    return std::unexpected("no player is in that slot");
  }

  return observer_.SelectPlayer(request.player(), state.xuid, state.generation, 1, nullptr,
                                **selection);
}

std::expected<void, std::string> Game::Watch(const players::WatchRequest& request) {
  // Find the camera's observer services, which aim it.
  if (auto target = Entity(request.target()); !target) {
    return std::unexpected(target.error());
  }

  // Find the camera pawn of the player's slot.
  auto selection = services_.Selection();
  if (!selection) {
    return std::unexpected(selection.error());
  }
  auto pawn = observer_.CameraPawnForSlot(request.player(), **selection);
  if (!pawn) {
    return std::unexpected(pawn.error());
  }

  // Read the pawn's observer services.
  auto services = services_.Field("CBasePlayerPawn", "m_pObserverServices");
  if (!services) {
    return std::unexpected(services.error());
  }
  unsigned char* observer = nullptr;
  if (!gameinterop::ReadNative(static_cast<unsigned char*>(*pawn) + services->offset, &observer,
                               sizeof(observer)) ||
      observer == nullptr) {
    return std::unexpected("the player's camera has no observer services");
  }

  // Resolve the observer target and mode fields.
  auto target_field = services_.Field("CPlayer_ObserverServices", "m_hObserverTarget");
  if (!target_field) {
    return std::unexpected(target_field.error());
  }
  auto mode_field = services_.Field("CPlayer_ObserverServices", "m_iObserverMode");
  if (!mode_field) {
    return std::unexpected(mode_field.error());
  }

  // Follow the target in its own view, OBS_MODE_IN_EYE, and replicate it.
  const uint32_t handle = request.target();
  const uint8_t in_eye = 2;
  if (!gameinterop::WriteNative(observer + target_field->offset, &handle, sizeof(handle)) ||
      !gameinterop::WriteNative(observer + mode_field->offset, &in_eye, sizeof(in_eye))) {
    return std::unexpected("cannot aim the spectator's camera");
  }
  if (!gameinterop::NotifyEntityStateChanged(*pawn)) {
    return std::unexpected("cannot replicate the spectator's camera");
  }
  return {};
}

std::expected<void, std::string> Game::Unwatch(const players::PlayerRequest& request) {
  // Return the player's camera to their hero.
  auto selection = services_.Selection();
  if (!selection) {
    return std::unexpected(selection.error());
  }
  auto camera = observer_.ReturnToHero(request.player(), **selection);
  if (!camera) {
    return std::unexpected(camera.error());
  }
  if (!*camera) {
    return {};
  }

  // Remove the camera entity the player watched through.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  static_cast<void>((*world)->RemoveEntity(*camera));
  return {};
}

std::expected<void, std::string> Game::Respawn(const players::PlayerRequest& request) {
  // Resolve the respawn call and the player's live pawn.
  auto respawn = services_.Respawn();
  if (!respawn) {
    return std::unexpected(respawn.error());
  }
  auto sample = Live(request.player());
  if (!sample) {
    return std::unexpected(sample.error());
  }
  return observer_.RespawnPlayer(request.player(), sample->session_generation, *respawn);
}

std::expected<void, std::string> Game::ClearItems(const players::PlayerRequest& request) {
  // Resolve the item tables and the player's live pawn.
  auto abilities = services_.Abilities();
  if (!abilities) {
    return std::unexpected(abilities.error());
  }
  auto items = services_.Items();
  if (!items) {
    return std::unexpected(items.error());
  }
  if (auto sample = Live(request.player()); !sample) {
    return std::unexpected(sample.error());
  }
  return observer_.ReconcileItems(request.player(), {}, **abilities, **items);
}

std::expected<void, std::string> Game::Freeze(const players::FreezeRequest& request) {
  // Check the player's hero is live, then freeze or release it.
  const auto slot = request.player();
  const bool frozen = !request.has_frozen() || request.frozen();
  if (auto sample = Live(slot); !sample) {
    return std::unexpected(sample.error());
  }
  if (auto done = observer_.SetPreparationFrozen(slot, frozen); !done) {
    return done;
  }

  // Remember the frozen heroes, so Clear can release them.
  if (frozen) {
    frozen_.insert(slot);
  } else {
    frozen_.erase(slot);
  }
  return {};
}

std::expected<void, std::string> Game::RestoreStamina(const players::PlayerRequest& request) {
  auto now = LatchTime();
  if (!now) {
    return std::unexpected(now.error());
  }
  if (auto sample = Live(request.player()); !sample) {
    return std::unexpected(sample.error());
  }
  return observer_.SetPracticeStamina(request.player(), *now);
}

std::expected<void, std::string> Game::SetStamina(const players::SetStaminaRequest& request) {
  auto now = LatchTime();
  if (!now) {
    return std::unexpected(now.error());
  }
  if (auto sample = Live(request.player()); !sample) {
    return std::unexpected(sample.error());
  }
  return observer_.SetPracticeStamina(request.player(), *now, request.stamina());
}

std::expected<float, std::string> Game::LatchTime() {
  // The simulation time keeps running through a pause, so a latch written
  // with it after a pause sits in the future of a clock without the paused
  // ticks, and the hero shows empty stamina until that clock catches up. A
  // latch without the paused ticks fills the hero under either clock. Without
  // game rules no pause has happened, and the two clocks agree.
  auto server = services_.Server();
  if (!server) {
    return std::unexpected(server.error());
  }

  // Read the server clock.
  auto now = (*server)->ReadClock();
  if (!now) {
    return std::unexpected(now.error());
  }

  // Without a match clock, the server time is the game time.
  auto clock = services_.Clock();
  if (!clock) {
    return now->current_time;
  }
  return (*clock)->GameTime(now->current_time, now->interval).value_or(now->current_time);
}

std::expected<void, std::string> Game::RefreshAbility(const abilities::RefreshAbilityRequest& request) {
  // Find the player's ability and clear its cooldown.
  auto ability = OwnedAbility(request.player(), request.ability());
  if (!ability) {
    return std::unexpected(ability.error());
  }
  ability->cooldown_start = 0;
  ability->cooldown_end = 0;
  return observer_.ApplyAbilityTimers(request.player(), std::span(&*ability, 1));
}

std::expected<abilities::AbilitiesResponse, std::string> Game::Abilities(const players::PlayerRequest& request) {
  // Resolve the ability table and the player's live abilities.
  const auto slot = request.player();
  auto definitions = services_.Abilities();
  if (!definitions) {
    return std::unexpected(definitions.error());
  }
  if (auto sample = Live(slot); !sample) {
    return std::unexpected(sample.error());
  }
  auto abilities = observer_.CurrentAbilitiesForSlot(slot);
  if (!abilities) {
    return std::unexpected(abilities.error());
  }

  // Answer each ability's name, slot and timers.
  abilities::AbilitiesResponse response;
  for (const auto& ability : *abilities) {
    // Name the ability, its slot and its entity.
    auto* out = response.add_abilities();
    if (auto definition = (*definitions)->Find(ability.subclass_id)) {
      out->set_name(definition->name);
    }
    out->set_slot(ability.slot);
    out->set_entity(ability.handle);

    // Then its upgrades, charges and timers.
    out->set_upgrades(ability.upgrade_info >> 16);
    out->set_charges(ability.charges);
    out->set_cooldown_end(ability.cooldown_end);
    out->set_id(ability.subclass_id);
    out->set_state(ability.upgrade_info);
    out->set_cooldown_start(ability.cooldown_start);
    out->set_recharge_start(ability.charge_recharge_start);
    out->set_recharge_end(ability.charge_recharge_end);
  }
  return response;
}

std::expected<void, std::string> Game::SetAbility(const abilities::AbilityOptions& request) {
  // Find the player's ability.
  auto ability = OwnedAbility(request.player(), request.ability());
  if (!ability) {
    return std::unexpected(ability.error());
  }

  // Apply the requested upgrades.
  if (request.has_upgrades()) {
    auto items = services_.Items();
    if (!items) {
      return std::unexpected(items.error());
    }
    const gameinterop::AbilityUpgrade upgrade{
        .subclass_id = ability->subclass_id,
        .slot = ability->slot,
        .upgrade_info = (ability->upgrade_info & 0xffff) | (request.upgrades() << 16),
    };
    if (auto applied = observer_.ApplyAbilityUpgrades(request.player(), std::span(&upgrade, 1),
                                                      (*items)->set_bits);
        !applied) {
      return applied;
    }
  }

  // Limit the ability's charges.
  if (request.has_max_charges()) {
    if (auto limited = LimitCharges(request.player(), ability->handle, request.max_charges());
        !limited) {
      return limited;
    }
  }

  // Stop when no timer was asked for.
  if (!request.has_charges() && !request.has_cooldown_end() && !request.has_recharge_end()) {
    return {};
  }

  // An upgrade can change the charges, so set them on the upgraded ability.
  ability = OwnedAbility(request.player(), request.ability());
  if (!ability) {
    return std::unexpected(ability.error());
  }
  if (request.has_charges()) {
    ability->charges = request.charges();
  }
  if (request.has_cooldown_end()) {
    ability->cooldown_end = request.cooldown_end();
  }
  if (request.has_recharge_end()) {
    ability->charge_recharge_end = request.recharge_end();
  }
  return observer_.ApplyAbilityTimers(request.player(), std::span(&*ability, 1));
}

std::expected<void, std::string> Game::LimitCharges(int32_t slot, uint32_t handle,
                                                    int32_t maximum) {
  // Create the charge limit once from the server module.
  if (!charge_limit_) {
    // Create the charge limit once.
    auto image = services_.Image();
    if (!image) {
      return std::unexpected(image.error());
    }
    auto slots = gameinterop::ResolveAbilityChargeSlots(**image);
    if (!slots) {
      return std::unexpected(slots.error());
    }
    auto created = gameinterop::AbilityChargeLimit::Create(slots->max_charges);
    if (!created) {
      return std::unexpected(created.error());
    }
    charge_limit_ = std::move(*created);
  }

  // Limit the live ability.
  auto* ability = observer_.AbilityInstance(slot, handle);
  if (ability == nullptr) {
    return std::unexpected("the ability is not live");
  }
  return charge_limit_->Set(ability, handle, maximum);
}

std::expected<void, std::string> Game::GiveItem(const abilities::GiveItemRequest& request) {
  // Resolve the item tables and the player's live pawn.
  auto definitions = services_.Abilities();
  if (!definitions) {
    return std::unexpected(definitions.error());
  }
  auto items = services_.Items();
  if (!items) {
    return std::unexpected(items.error());
  }
  if (auto sample = Live(request.player()); !sample) {
    return std::unexpected(sample.error());
  }

  // Grant the item to the player's hero.
  auto item = observer_.GrantItem(
      request.player(), gameinterop::MakeMemberName(request.item()).hash, **definitions, **items);
  if (!item) {
    return std::unexpected(item.error());
  }
  return {};
}

std::expected<void, std::string> Game::ReplaceAbility(const abilities::ReplaceAbilityRequest& request) {
  // Check the slot and resolve the ability tables.
  if (request.index() > UINT16_MAX) {
    return std::unexpected("the ability slot is out of range");
  }
  auto definitions = services_.Abilities();
  if (!definitions) {
    return std::unexpected(definitions.error());
  }
  auto slots = services_.Slots();
  if (!slots) {
    return std::unexpected(slots.error());
  }
  auto create = services_.CreateAbility();
  if (!create) {
    return std::unexpected(create.error());
  }

  // Replace the ability on the player's live hero.
  if (auto sample = Live(request.player()); !sample) {
    return std::unexpected(sample.error());
  }
  return (*slots)->Replace(observer_, request.player(), static_cast<uint16_t>(request.index()),
                           gameinterop::MakeMemberName(request.ability()).hash, **definitions,
                           *create);
}

std::expected<void, std::string> Game::HoldModifier(const abilities::HoldModifierRequest& request) {
  // Releasing a hold drops the slot's claim on the modifier.
  const auto& key = request.modifier();
  if (request.has_active() && !request.active()) {
    ReleaseModifier(request.player(), key);
    return {};
  }

  // A held modifier is named by its ability and its own name.
  if (key.find('/') == std::string::npos) {
    return std::unexpected("name the modifier as <ability>/<modifier>");
  }

  // Resolve the modifier once for every slot that holds it.
  if (!modifiers_.contains(key)) {
    // Resolve the modifier once for its key.
    auto image = services_.Image();
    if (!image) {
      return std::unexpected(image.error());
    }
    auto schema = services_.Schema();
    if (!schema) {
      return std::unexpected(schema.error());
    }
    auto modifier = gameinterop::AbilityModifier::Resolve(**image, *schema, key, -1);
    if (!modifier) {
      return std::unexpected(modifier.error());
    }
    modifiers_.emplace(key, *modifier);
  }
  held_modifiers_.emplace(request.player(), key);

  // A hold that cannot start now ends, so the mod learns why.
  auto renewed = RenewModifier(request.player(), key);
  if (!renewed) {
    ReleaseModifier(request.player(), key);
  }
  return renewed;
}

std::expected<void, std::string> Game::GiveModifier(const abilities::GiveModifierRequest& request) {
  // Split the modifier's name at its ability.
  const auto& key = request.modifier();
  const auto slash = key.find('/');
  if (slash == std::string::npos) {
    return std::unexpected("name the modifier as <ability>/<modifier>");
  }

  // Resolve the server module, the schema and the hero's abilities.
  auto image = services_.Image();
  if (!image) {
    return std::unexpected(image.error());
  }
  auto schema = services_.Schema();
  if (!schema) {
    return std::unexpected(schema.error());
  }
  if (auto sample = Live(request.player()); !sample) {
    return std::unexpected(sample.error());
  }
  auto owned = observer_.CurrentAbilitiesForSlot(request.player());
  if (!owned) {
    return std::unexpected(owned.error());
  }

  // The modifier's own ability applies it when the hero owns that ability;
  // otherwise the hero's first ability does.
  auto source = gameinterop::MakeMemberName(std::string_view(key).substr(0, slash)).hash;
  if (std::ranges::none_of(*owned,
                           [&](const auto& ability) { return ability.subclass_id == source; })) {
    const auto first = std::ranges::find(*owned, 0, &gameinterop::PawnObserver::Ability::slot);
    if (first == owned->end()) {
      return std::unexpected("the hero has no ability to apply the modifier");
    }
    source = first->subclass_id;
  }

  // The modifier takes its duration when attached, so the shared definition
  // gets its own duration back right away.
  auto modifier = gameinterop::AbilityModifier::Resolve(**image, *schema, key, request.seconds());
  if (!modifier) {
    return std::unexpected(modifier.error());
  }
  auto attached = modifier->Attach(observer_, request.player(), source);
  modifier->Restore();
  return attached;
}

std::expected<gameinterop::PawnObserver::Ability, std::string> Game::OwnedAbility(
    int32_t slot, std::string_view name) {
  // Resolve the ability table and the player's live abilities.
  auto definitions = services_.Abilities();
  if (!definitions) {
    return std::unexpected(definitions.error());
  }
  if (auto sample = Live(slot); !sample) {
    return std::unexpected(sample.error());
  }
  auto abilities = observer_.CurrentAbilitiesForSlot(slot);
  if (!abilities) {
    return std::unexpected(abilities.error());
  }

  // Find the owned ability with that name.
  for (const auto& ability : *abilities) {
    const auto definition = (*definitions)->Find(ability.subclass_id);
    if (definition && definition->name == name) {
      return ability;
    }
  }
  return std::unexpected("the hero has no ability named " + std::string(name));
}

std::expected<void, std::string> Game::Teleport(const movement::TeleportRequest& request) {
  // Resolve the teleport call and the player's live hero.
  auto teleport = services_.Teleport();
  if (!teleport) {
    return std::unexpected(teleport.error());
  }
  if (auto sample = Live(request.player()); !sample) {
    return std::unexpected(sample.error());
  }

  // Teleport the hero, with a velocity when the request sets one.
  const auto position = Floats(request.position());
  const auto angles = Floats(request.facing());
  auto* pawn = observer_.PawnForSlot(request.player());
  if (request.has_velocity()) {
    gameinterop::TeleportEntity(pawn, position, angles, Floats(request.velocity()));
  } else {
    (*teleport)(nullptr, pawn, position.data(), angles.data());
  }

  // A teleport recomputes the hero's modifier states, dropping held ones.
  ApplyHolds();
  return {};
}

std::expected<void, std::string> Game::MovePlayer(const movement::MovePlayerRequest& request) {
  // Resolve the motion setters and the player's live hero.
  auto motion = services_.Motion();
  if (!motion) {
    return std::unexpected(motion.error());
  }
  const auto slot = request.player();
  if (auto sample = Live(slot); !sample) {
    return std::unexpected(sample.error());
  }

  // Convert the requested pose to the game's floats.
  const auto position = Floats(request.position());
  const auto facing = Floats(request.facing());
  const auto velocity = Floats(request.velocity());

  // The aim takes the whole facing; tilting the body by pitch would lean the
  // whole hero.
  auto* pawn = observer_.PawnForSlot(slot);
  if (auto aimed = observer_.SetEyeAngles(pawn, facing); !aimed) {
    return aimed;
  }
  const std::array<float, 3> body{0, facing[1], 0};
  (*motion)->set_angles(pawn, body.data());
  (*motion)->set_velocity(pawn, velocity.data());
  (*motion)->set_origin(pawn, position.data());
  return {};
}

std::expected<void, std::string> Game::Steer(const movement::SteerRequest& request) {
  // Without steering, the slot's bot goes back to its own commands.
  const auto slot = request.player();
  if (!request.has_steering()) {
    steers_.erase(slot);
    return services_.Steps();
  }

  // Steer the live hero, and drop the steering if the movement hook fails.
  if (auto sample = Live(slot); !sample) {
    return std::unexpected(sample.error());
  }
  steers_.insert_or_assign(
      slot, Steered{.pawn = observer_.PawnForSlot(slot), .steering = request.steering()});
  auto steps = services_.Steps();
  if (!steps) {
    steers_.erase(slot);
  }
  return steps;
}

std::expected<void, std::string> Game::Drive(const movement::DriveRequest& request) {
  // A request without a command stops the drive.
  const auto slot = request.player();
  if (!request.has_command()) {
    drives_.erase(slot);
    return services_.Steps();
  }

  // Only a bot can be driven, through the shared hook.
  if (!gameinterop::ConnectionTracker::StateForSlot(slot).is_bot) {
    return std::unexpected("no bot plays in that slot");
  }
  if (auto installed = services_.Drive(); !installed) {
    return std::unexpected(installed.error());
  }

  // Find the slot's drive.
  const auto& command = request.command();
  const bool fresh = !drives_.contains(slot);
  auto& drive = drives_[slot];

  // A press waits for the next command even when the mod drives again first.
  const auto pressed = drive.pressed | command.pressed();
  drive = {.angles = Floats(command.facing()),
           .forward = std::clamp(command.forward(), -1.0f, 1.0f),
           .left = std::clamp(command.left(), -1.0f, 1.0f),
           .held = command.held(),
           .pressed = pressed};

  // A new drive starts the movement step; a failed start drops it.
  if (!fresh) {
    return {};
  }
  auto steps = services_.Steps();
  if (!steps) {
    drives_.erase(slot);
  }
  return steps;
}

}  // namespace modlock::wasm

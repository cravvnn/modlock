#include <algorithm>
#include <array>
#include <cstring>
#include <iostream>
#include <type_traits>
#include <utility>
#include <vector>

#include "modlock/gameinterop/console_variables.h"
#include "modlock/gameinterop/player_selection.h"
#include "wasm/game.h"
#include "wasm/game_internal.h"

namespace modlock::wasm {

namespace {

// Remap moves the state of remap's from buttons to its to buttons.
void Remap(const RemapInputRequest& remap, gameinterop::AbilityInputHook::Input& input) {
  if (remap.from() == 0) {
    return;
  }
  const bool held = input.buttons[0] & remap.from();
  for (auto& state : input.buttons) {
    const bool set = state & remap.from();
    state &= ~remap.from();
    if (set) {
      state |= remap.to();
    }
  }
  if (held && remap.repeat()) {
    input.buttons[1] |= remap.to();
  }
}

// kMoveKeys are IN_FORWARD, IN_BACK, IN_MOVELEFT and IN_MOVERIGHT, which the
// game holds beside the movement axes.
constexpr uint64_t kMoveKeys = (1u << 3) | (1u << 4) | (1u << 9) | (1u << 10);

uint64_t MoveKeys(float forward, float left) {
  // Hold the key for each axis direction.
  uint64_t keys = 0;
  if (forward > 0) {
    keys |= 1u << 3;
  }
  if (forward < 0) {
    keys |= 1u << 4;
  }
  if (left > 0) {
    keys |= 1u << 9;
  }
  if (left < 0) {
    keys |= 1u << 10;
  }
  return keys;
}

}  // namespace

std::expected<const gameinterop::EngineServer*, std::string> GameServices::Server() {
  if (!server_) {
    auto server = gameinterop::EngineServer::Resolve();
    if (!server) {
      return std::unexpected(server.error());
    }
    server_ = *server;
  }
  return &*server_;
}

std::expected<const gameinterop::NativeUserMessages*, std::string> GameServices::Messages() {
  if (!messages_) {
    auto messages = gameinterop::NativeUserMessages::TryCreate();
    if (!messages) {
      return std::unexpected(messages.error());
    }
    messages_ = *messages;
  }
  return &*messages_;
}

std::expected<void, std::string> GameServices::ServerCommand(std::string command) {
  // Hold the line until the world starts.
  command += '\n';
  if (!started_) {
    pending_commands_.push_back(std::move(command));
    return {};
  }

  // Resolve the engine server that runs console lines.
  auto server = Server();
  if (!server) {
    return std::unexpected(server.error());
  }

  // A line that sets a variable may name a development-only or cheat-protected
  // one, which the console otherwise refuses. Exposing the first word of any
  // other line finds no variable and changes nothing.
  if (auto variables = gameinterop::ConsoleVariables::Resolve()) {
    const auto name = command.substr(0, command.find_first_of(" \t\n"));
    static_cast<void>(variables->AllowServerChanges(name.c_str()));
  }

  // Run the line on the server console.
  if (!(*server)->ServerCommand(command.c_str())) {
    return std::unexpected("the server console is unavailable");
  }
  return {};
}

void GameServices::WorldStarting() {
  started_ = true;
  auto pending = std::exchange(pending_commands_, {});
  for (auto& command : pending) {
    if (auto run = ServerCommand(std::move(command)); !run) {
      std::cerr << "server command: " << run.error() << '\n';
    }
  }
}

void GameServices::WorldReady() {
  // Mods loaded after the world started never saw it start.
  if (!started_) {
    WorldStarting();
  }
  world_ready_ = true;
  if (input_wanted_) {
    Input();
  }
  if (impacts_wanted_) {
    Projectiles();
  }
}

std::expected<const gameinterop::ModuleImage*, std::string> GameServices::Image() {
  if (!image_) {
    auto image = gameinterop::MappedModuleImage::ForModule(L"server.dll");
    if (!image) {
      return std::unexpected(image.error());
    }
    image_ = *image;
  }
  return &*image_;
}

std::expected<void*, std::string> GameServices::Schema() {
  return gameinterop::ResolveSchemaSystem();
}

namespace {

// Keep stores a resolved value in its cache, or passes the failure on.
template <typename Value, typename Resolved>
std::expected<void, std::string> Keep(Value& cache, Resolved resolved) {
  if (!resolved) {
    return std::unexpected(resolved.error());
  }
  cache = std::move(*resolved);
  return {};
}

}  // namespace

template <typename Value, typename Resolve>
std::expected<void, std::string> GameServices::Cache(Value& cache, Resolve resolve) {
  // A filled cache needs nothing more.
  if (cache) {
    return {};
  }

  // Resolve from the server image, with the schema when resolve takes it.
  auto image = Image();
  if (!image) {
    return std::unexpected(image.error());
  }
  if constexpr (std::is_invocable_v<Resolve, const gameinterop::ModuleImage&, void*>) {
    auto schema = Schema();
    if (!schema) {
      return std::unexpected(schema.error());
    }
    return Keep(cache, resolve(**image, *schema));
  } else {
    return Keep(cache, resolve(**image));
  }
}

std::expected<const gameinterop::HeroDefinitions*, std::string> GameServices::Heroes() {
  if (auto cached = Cache(heroes_, &gameinterop::HeroDefinitions::Resolve); !cached) {
    return std::unexpected(cached.error());
  }
  return &*heroes_;
}

std::expected<const gameinterop::PlayerSelectionCalls*, std::string> GameServices::Selection() {
  if (auto cached = Cache(selection_, &gameinterop::PlayerSelectionCalls::Resolve); !cached) {
    return std::unexpected(cached.error());
  }
  return &*selection_;
}

std::expected<const gameinterop::AbilityDefinitions*, std::string> GameServices::Abilities() {
  if (auto cached = Cache(abilities_, &gameinterop::AbilityDefinitions::Resolve); !cached) {
    return std::unexpected(cached.error());
  }
  return &*abilities_;
}

std::expected<const gameinterop::ItemFunctions*, std::string> GameServices::Items() {
  if (auto cached = Cache(items_, &gameinterop::ItemFunctions::Resolve); !cached) {
    return std::unexpected(cached.error());
  }
  return &*items_;
}

std::expected<const gameinterop::AbilitySlots*, std::string> GameServices::Slots() {
  if (auto cached = Cache(slots_, &gameinterop::AbilitySlots::Resolve); !cached) {
    return std::unexpected(cached.error());
  }
  return &*slots_;
}

std::expected<gameinterop::CreateAbility, std::string> GameServices::CreateAbility() {
  if (auto cached = Cache(create_ability_, &gameinterop::ResolveCreateAbility); !cached) {
    return std::unexpected(cached.error());
  }
  return create_ability_;
}

std::expected<gameinterop::RespawnPawn, std::string> GameServices::Respawn() {
  if (auto cached = Cache(respawn_, &gameinterop::ResolveRespawnPawn); !cached) {
    return std::unexpected(cached.error());
  }
  return respawn_;
}

std::expected<gameinterop::ResetHeroPawn, std::string> GameServices::ResetHero() {
  if (auto cached = Cache(reset_hero_, &gameinterop::ResolveResetHeroPawn); !cached) {
    return std::unexpected(cached.error());
  }
  return reset_hero_;
}

std::expected<gameinterop::TeleportClientCamera, std::string> GameServices::Teleport() {
  if (auto cached = Cache(teleport_, &gameinterop::ResolveTeleportClientCamera); !cached) {
    return std::unexpected(cached.error());
  }
  return teleport_;
}

std::expected<const gameinterop::PawnMotion*, std::string> GameServices::Motion() {
  if (auto cached = Cache(motion_, &gameinterop::ResolvePawnMotion); !cached) {
    return std::unexpected(cached.error());
  }
  return &*motion_;
}

std::expected<const gameinterop::BotCreation*, std::string> GameServices::Bots() {
  if (!bots_) {
    // Resolve the bot calls once.
    auto image = Image();
    if (!image) {
      return std::unexpected(image.error());
    }
    auto bots = gameinterop::BotCreation::Resolve(**image);
    if (!bots) {
      return std::unexpected(bots.error());
    }
    bots_ = *bots;

    // Without the guard a solo bot match clears a bot's hero that a human
    // plays; the bots still join, so a failure costs only mirrored heroes.
    if (auto guard = gameinterop::BotHeroGuard::Install(**image)) {
      bot_heroes_ = std::move(*guard);
    } else {
      std::cerr << "mod bots: " << guard.error() << '\n';
    }
  }
  return &*bots_;
}

std::expected<render::WorldTextGameFactory*, std::string> GameServices::Text() {
  // Build the text factory once from the server module.
  if (auto cached = Cache(text_, &render::WorldTextGameFactory::TryCreate); !cached) {
    return std::unexpected(cached.error());
  }

  // Each world has its own entity system.
  auto entities = gameinterop::ResolveLiveEntitySystem();
  if (!entities) {
    return std::unexpected(entities.error());
  }
  text_->SetEntitySystem(*entities);
  return text_.get();
}

std::expected<render::WorldEffectGameFactory*, std::string> GameServices::Effects() {
  // Build the effect factory once from the server module.
  if (auto cached = Cache(effects_, &render::WorldEffectGameFactory::TryCreate); !cached) {
    return std::unexpected(cached.error());
  }

  // Each world has its own entity system.
  auto entities = gameinterop::ResolveLiveEntitySystem();
  if (!entities) {
    return std::unexpected(entities.error());
  }
  effects_->SetEntitySystem(*entities);
  return effects_.get();
}

std::expected<const gameinterop::NativeDamage*, std::string> GameServices::Damage() {
  if (auto cached = Cache(damage_, &gameinterop::NativeDamage::Resolve); !cached) {
    return std::unexpected(cached.error());
  }
  return &*damage_;
}

std::expected<gameinterop::ModifyCurrency, std::string> GameServices::Currency() {
  if (auto cached = Cache(currency_, &gameinterop::ResolveModifyCurrency); !cached) {
    return std::unexpected(cached.error());
  }
  return currency_;
}

std::expected<const gameinterop::NativeSound*, std::string> GameServices::Sound() {
  if (auto cached = Cache(sound_, &gameinterop::NativeSound::TryCreate); !cached) {
    return std::unexpected(cached.error());
  }
  return &*sound_;
}

std::expected<const gameinterop::GamePause*, std::string> GameServices::Pause() {
  if (auto cached = Cache(pause_, &gameinterop::GamePause::Resolve); !cached) {
    return std::unexpected(cached.error());
  }
  return &*pause_;
}

std::expected<gameinterop::MatchClock*, std::string> GameServices::Clock() {
  if (auto cached = Cache(clock_, &gameinterop::MatchClock::Resolve); !cached) {
    return std::unexpected(cached.error());
  }
  return &*clock_;
}

std::expected<void*, std::string> GameServices::Rules() {
  // Resolve the server module the rules must live in.
  auto image = Image();
  if (!image) {
    return std::unexpected(image.error());
  }

  // Find the game's pointer to its current rules once.
  if (!rules_) {
    auto current = gameinterop::ResolveCurrentGameRules(**image);
    if (!current) {
      return std::unexpected(current.error());
    }
    rules_ = *current;
  }

  // Read the rules through that pointer.
  void* rules = nullptr;
  std::memcpy(&rules, rules_, sizeof(rules));
  if (!rules) {
    return std::unexpected("the game rules are not active");
  }

  // A rules object whose table leaves the server module is stale.
  uintptr_t table = 0;
  std::memcpy(&table, rules, sizeof(table));
  if (table < (*image)->base() || table - (*image)->base() >= (*image)->image_bytes().size()) {
    return std::unexpected("the game rules leave the server module");
  }
  return rules;
}

std::expected<gameinterop::KothRules*, std::string> GameServices::Rift() {
  if (auto cached = Cache(rift_, &gameinterop::KothRules::Resolve); !cached) {
    return std::unexpected(cached.error());
  }
  return &*rift_;
}

std::expected<gameinterop::SchemaField, std::string> GameServices::Field(
    const std::string& class_name, const std::string& field) {
  // Answer a field resolved before from the cache.
  FieldKey key{class_name, field};
  if (auto found = fields_.find(key); found != fields_.end()) {
    return found->second;
  }

  // Resolve the field in the server's schema and cache it.
  auto schema = Schema();
  if (!schema) {
    return std::unexpected(schema.error());
  }
  auto resolved =
      gameinterop::SchemaFieldOf(*schema, "server.dll", class_name.c_str(), field.c_str());
  if (!resolved) {
    return std::unexpected(class_name + "." + field + ": " + resolved.error());
  }
  fields_.emplace(std::move(key), *resolved);
  return *resolved;
}

bool GameServices::Derives(const void* class_info, const std::string& class_name) {
  // Answer a class resolved before from the cache.
  auto key = std::pair{class_info, class_name};
  if (auto found = derives_.find(key); found != derives_.end()) {
    return found->second;
  }

  // Walk the class's bases and cache the answer.
  const bool derives = gameinterop::SchemaClassDerivesFrom(class_info, class_name);
  derives_.emplace(std::move(key), derives);
  return derives;
}

std::expected<GameServices::ModifierLayout, std::string> GameServices::Modifiers() {
  // Answer the layout resolved before.
  if (modifiers_) {
    return *modifiers_;
  }

  // Resolve the modifier property and its two state masks.
  auto property = Field("CBaseEntity", "m_pModifierProp");
  if (!property) {
    return std::unexpected(property.error());
  }
  auto mask = Field("CModifierProperty", "m_bvEnabledStateMask");
  if (!mask) {
    return std::unexpected(mask.error());
  }
  auto predicted = Field("CModifierProperty", "m_bvEnabledPredictedStateMask");
  if (!predicted) {
    return std::unexpected(predicted.error());
  }

  // Cache the offsets every modifier state read uses.
  modifiers_ = ModifierLayout{
      .property = property->offset,
      .mask = mask->offset,
      .predicted_mask = predicted->offset,
      .mask_size = std::min(mask->size, predicted->size),
  };
  return *modifiers_;
}

std::expected<int64_t, std::string> GameServices::ModifierState(const std::string& name) {
  // Answer a state resolved before from the cache.
  if (auto found = modifier_states_.find(name); found != modifier_states_.end()) {
    return found->second;
  }

  // Resolve the state in the server's enum and cache it.
  auto schema = Schema();
  if (!schema) {
    return std::unexpected(schema.error());
  }
  auto value =
      gameinterop::SchemaEnumValueOf(*schema, "server.dll", "EModifierState", name.c_str());
  if (!value) {
    return std::unexpected(name + ": " + value.error());
  }
  modifier_states_.emplace(name, *value);
  return *value;
}

void GameServices::Input() {
  // Install the hook once the world is ready to take input.
  input_wanted_ = true;
  if (input_ || !world_ready_) {
    return;
  }

  // Resolve the server module and schema the hook needs.
  auto image = Image();
  auto schema = Schema();
  if (!image || !schema) {
    return;
  }

  // Bots take input too, so a mod can press their buttons.
  auto input = gameinterop::AbilityInputHook::Install(
      **image, *schema,
      [this](gameinterop::AbilityInputHook::Input& input) { return Filter(input); },
      [](std::string error) { std::cerr << "mod input: " << error << '\n'; }, true);
  if (!input) {
    std::cerr << "mod input: " << input.error() << '\n';
    return;
  }
  input_ = std::move(*input);
}

uint64_t GameServices::Filter(gameinterop::AbilityInputHook::Input& input) {
  // A press is a button held now that changed for this input; a release is
  // one that changed and is up.
  const uint64_t pressed = input.buttons[0] & input.buttons[1];
  const uint64_t released = ~input.buttons[0] & input.buttons[1];
  uint64_t blocked = 0;
  for (auto* game : games_) {
    // Merge the game's blocks and record its watched changes.
    blocked |= game->blocked_;
    if (auto found = game->slot_blocked_.find(input.slot); found != game->slot_blocked_.end()) {
      blocked |= found->second;
    }
    if ((pressed | released) & game->watched_ && game->changes_.size() < kMaxChanges) {
      game->changes_.push_back({input.slot, pressed & game->watched_, released & game->watched_});
    }

    // Take the game's pending press for this slot.
    auto press = game->presses_.find(input.slot);
    if (press == game->presses_.end()) {
      continue;
    }

    // A press is down and newly changed for this one input.
    input.buttons[0] |= press->second;
    input.buttons[1] |= press->second;
    input.buttons[2] &= ~press->second;
    game->presses_.erase(press);
  }
  for (auto* game : games_) {
    Remap(game->remap_, input);
  }
  return blocked;
}

void GameServices::Projectiles() {
  // Install the hook once the world is ready for impacts.
  impacts_wanted_ = true;
  if (impacts_ || !world_ready_) {
    return;
  }

  // Resolve the server module the hook patches.
  auto image = Image();
  if (!image) {
    return;
  }

  // Hook impacts, keeping the hook only when it installs.
  auto hook = gameinterop::ProjectileImpactHook::Install(
      **image, [this](void* entity, const auto& contact, const auto& native) {
        Impact(entity, contact, native);
      });
  if (!hook) {
    std::cerr << "mod projectiles: " << hook.error() << '\n';
    return;
  }
  impacts_ = std::move(*hook);
}

std::expected<void, std::string> GameServices::Transmit() {
  // Install the transmit hook once.
  if (transmit_) {
    return {};
  }
  auto hook =
      gameinterop::TransmitHook::Install([this](gameinterop::TransmitSet& set) { Withhold(set); });
  if (!hook) {
    return std::unexpected(hook.error());
  }
  transmit_ = std::move(*hook);
  return {};
}

std::expected<void, std::string> GameServices::Drive() {
  // Install once, against the server image.
  if (drive_) {
    return {};
  }
  auto image = Image();
  if (!image) {
    return std::unexpected(image.error());
  }
  auto hook =
      gameinterop::BotDrive::Install(**image, [this](int32_t slot) { return Driven(slot); });
  if (!hook) {
    return std::unexpected(hook.error());
  }
  drive_ = std::move(*hook);
  return {};
}

std::optional<gameinterop::BotDrive::Command> GameServices::Driven(int32_t slot) {
  for (auto* game : games_) {
    // The first game driving the slot answers; its press counts once.
    auto found = game->drives_.find(slot);
    if (found == game->drives_.end()) {
      continue;
    }
    const auto command = found->second;
    found->second.pressed = 0;
    return command;
  }
  return std::nullopt;
}

void GameServices::Withhold(gameinterop::TransmitSet& set) {
  // A text object's entity changes with its words, so each update reads the
  // current handles.
  for (auto* game : games_) {
    auto hidden = game->hidden_.find(set.Slot());
    if (hidden == game->hidden_.end()) {
      continue;
    }
    for (const auto id : hidden->second) {
      auto found = game->objects_.find(id);
      if (found == game->objects_.end()) {
        continue;
      }
      auto handle = std::visit([](const auto& object) { return object->Handle(); }, found->second);
      if (handle) {
        set.Withhold(*handle);
      }
    }
  }

  // The engine's pass already set the bits of entities sent to every client,
  // TransmitAlways ones included, so clearing them here still withholds them.
  for (auto* game : games_) {
    for (const auto& [entity, slots] : game->audiences_) {
      set.WithholdUnlessIn(entity, slots);
    }
  }
}

void GameServices::Impact(void* entity, const gameinterop::TraceResult& contact,
                          const gameinterop::ProjectileImpactHook::NativeImpact& native) {
  // Collect the games that watch this projectile.
  std::vector<std::pair<Game*, ImpactEvent>> watching;
  bool keep = false;
  for (auto* game : games_) {
    auto event = game->Watching(entity, contact);
    if (!event) {
      continue;
    }
    keep |= game->projectiles_.keep_momentum();
    watching.emplace_back(game, std::move(*event));
  }
  if (watching.empty()) {
    native();
    return;
  }

  // The game's explosion replaces the motion of heroes near it, even with no
  // push of its own.
  std::vector<Game::Hero> heroes;
  if (keep) {
    heroes = watching.front().first->Heroes();
  }
  native();
  for (const auto& hero : heroes) {
    if (hero.velocity) {
      gameinterop::SetEntityVelocity(hero.pawn, *hero.velocity);
    }
  }

  // Tell each watching game, if it is still loaded.
  for (auto& [game, event] : watching) {
    if (games_.contains(game)) {
      game->Impacted(event);
    }
  }
}

std::expected<void, std::string> GameServices::StepWith(gameinterop::MovementHook::Handler step) {
  // Allow one mod's movement handler at a time.
  if (step && step_) {
    return std::unexpected("another mod's movement runs");
  }
  step_ = std::move(step);

  // Install or remove the movement hook to match.
  auto steps = Steps();
  if (!steps) {
    step_ = nullptr;
  }
  return steps;
}

std::expected<void, std::string> GameServices::Steps() {
  // Keep the hook only while a mod or a steered bot moves heroes.
  const bool needed = step_ || std::ranges::any_of(games_, [](const Game* game) {
                        return game->Moving() || !game->steers_.empty() || !game->drives_.empty();
                      });
  if (!needed) {
    steps_.reset();
    commands_.clear();
    return {};
  }

  // Install the hook the first time a mover needs it.
  if (steps_) {
    return {};
  }
  auto hook = gameinterop::MovementHook::Install(
      gameinterop::MovementHook::Module::kServer,
      [this](gameinterop::MovementCall& call) { return Step(call); });
  if (!hook) {
    return std::unexpected(hook.error());
  }
  steps_ = std::move(*hook);
  return {};
}

bool GameServices::Step(gameinterop::MovementCall& call) {
  // Note the ground the last step stood on.
  using gameinterop::MovementHook;
  if (call.ground_entity != MovementHook::kNoGround) {
    ground_ = call.ground_entity;
  }

  // Record the pawn's command for the mod to read.
  if (call.pawn) {
    // The tick's first command fixes its starting ground; later commands add
    // their presses and leave the last buttons and axes.
    auto& [command, stepped] = commands_[call.pawn];
    if (!std::exchange(stepped, true)) {
      command.set_grounded(call.ground_entity != MovementHook::kNoGround);
    }
    command.set_held(call.buttons[0]);
    command.set_changed(command.changed() | call.buttons[1]);
    command.set_scroll(command.scroll() | call.buttons[2]);
    command.set_forward(call.forward);
    command.set_left(call.left);
    command.set_up(call.up);
  }

  // Steer a bot that replays a recorded command.
  for (auto* game : games_) {
    for (auto& [slot, steered] : game->steers_) {
      // Steer the pawn this game replays.
      if (steered.pawn != call.pawn) {
        continue;
      }

      // The recorded command drives the game's own movement, abilities and
      // animation for this step.
      auto& steering = steered.steering;
      auto& command = *steering.mutable_command();
      call.buttons = {command.held(), command.changed(), command.scroll()};
      call.forward = command.forward();
      call.left = command.left();
      call.up = command.up();

      // The recorded view steers the step and becomes the hero's aim; its yaw
      // turns the body, as Game::MovePlayer does. The bot's own command set
      // both before this step, and the eye angles alone leave the body at the
      // bot's yaw, so the hero would run sideways.
      call.angles = Floats(steering.facing());
      static_cast<void>(game->observer_.SetEyeAngles(call.pawn, call.angles));
      if (auto motion = Motion()) {
        const std::array<float, 3> body{0, call.angles[1], 0};
        (*motion)->set_angles(call.pawn, body.data());
      }

      // A press counts once, though the command stays for the ticks after.
      command.set_changed(0);
      command.set_scroll(0);

      // The step then ends exactly on the recorded pose, on the ground the
      // heroes stand on when its own step left it in the air.
      call.origin = Floats(steering.position());
      call.velocity = Floats(steering.velocity());
      const auto ground = call.ground_entity;
      if (!steering.grounded()) {
        call.ground_entity = MovementHook::kNoGround;
      } else if (ground == MovementHook::kNoGround) {
        call.ground_entity = ground_;
      }
      if (call.ground_entity != ground) {
        static_cast<void>(gameinterop::NotifyEntityStateChanged(call.pawn));
      }
      return true;
    }
  }

  // A driven bot's axes reach movement here: the game rebuilds them, and the
  // movement keys, from its own state after ProcessUsercmds, which keeps the
  // buttons and view the drive wrote there. The step stays native.
  for (auto* game : games_) {
    for (auto& [slot, drive] : game->drives_) {
      // Drive the pawn this game drives.
      if (game->observer_.PawnForSlot(slot) != call.pawn) {
        continue;
      }
      call.forward = drive.forward;
      call.left = drive.left;
      call.buttons[0] = (call.buttons[0] & ~kMoveKeys) | MoveKeys(drive.forward, drive.left);
      return false;
    }
  }
  return step_ && step_(call);
}

std::optional<MovementCommand> GameServices::Command(const void* pawn) {
  // Find the pawn's command for this tick.
  const auto found = commands_.find(pawn);
  if (found == commands_.end()) {
    return std::nullopt;
  }

  // Hand it over, keeping the held buttons and clearing the presses.
  auto& [command, stepped] = found->second;
  auto taken = command;
  command.set_changed(0);
  command.set_scroll(0);
  stepped = false;
  return taken;
}

}  // namespace modlock::wasm

#include <algorithm>
#include <array>
#include <deque>
#include <utility>
#include <vector>

#include "modlock/gameinterop/connection_tracker.h"
#include "modlock/gameinterop/keyvalues.h"
#include "wasm/game.h"
#include "wasm/game_internal.h"

namespace modlock::wasm {

namespace {

// Value converts an entity value to the world's form, or nothing when it has
// none. The result borrows value's text.
std::optional<gameinterop::EntityValue> Value(const EntityValue& value) {
  switch (value.value_case()) {
    case EntityValue::kBoolean:
      return value.boolean();
    case EntityValue::kInteger:
      return value.integer();
    case EntityValue::kNumber:
      return value.number();
    case EntityValue::kText:
      return std::string_view(value.text());
    case EntityValue::kVector: {
      const auto [x, y, z] = Floats(value.vector());
      return gameinterop::KeyValueVector{x, y, z};
    }
    case EntityValue::kColor: {
      const uint32_t color = value.color();
      return gameinterop::KeyValueColor{
          static_cast<uint8_t>(color >> 24), static_cast<uint8_t>(color >> 16),
          static_cast<uint8_t>(color >> 8), static_cast<uint8_t>(color)};
    }
    case EntityValue::VALUE_NOT_SET:
      break;
  }
  return std::nullopt;
}

// KeyValues converts spawn key values to the world's form, writing addon in
// place of each {addon} in a text value. The result borrows the text of pairs,
// and of filled, which holds each text value it rewrote.
std::expected<std::vector<gameinterop::EntityKeyValue>, std::string> KeyValues(
    const google::protobuf::RepeatedPtrField<KeyValue>& pairs, std::string_view addon,
    std::deque<std::string>& filled) {
  // Convert each pair, failing on a pair with no value.
  constexpr std::string_view token = "{addon}";
  std::vector<gameinterop::EntityKeyValue> result;
  result.reserve(pairs.size());
  for (const auto& pair : pairs) {
    // Convert the pair's value.
    auto value = Value(pair.value());
    if (!value) {
      return std::unexpected("key value " + pair.key() + " has no value");
    }

    // Name the mod's content addon in a text value.
    const auto* text = std::get_if<std::string_view>(&*value);
    if (text && !addon.empty() && text->find(token) != std::string_view::npos) {
      auto& rewritten = filled.emplace_back(*text);
      for (auto at = rewritten.find(token); at != std::string::npos;
           at = rewritten.find(token, at + addon.size())) {
        rewritten.replace(at, token.size(), addon);
      }
      *value = std::string_view(rewritten);
    }
    result.push_back({.key = pair.key(), .value = *value});
  }
  return result;
}

// Alive answers whether the entity a call acted on is still there.
std::expected<AliveResponse, std::string> Alive(std::expected<bool, std::string> alive) {
  if (!alive) {
    return std::unexpected(alive.error());
  }
  AliveResponse response;
  response.set_alive(*alive);
  return response;
}

}  // namespace

std::expected<ObjectResponse, std::string> Game::CreateModel(const ModelOptions& request) {
  // Resolve the effect factory.
  auto effects = services_.Effects();
  if (!effects) {
    return std::unexpected(effects.error());
  }

  // Create the model and keep it as one of the mod's objects.
  render::WorldModelSettings settings{
      .resource = request.resource(),
      .origin = request.position(),
      .angles = Floats(request.facing()),
      .scale = request.scale() == 0 ? 1 : request.scale(),
      // The factory reads red from the low byte, as 0xAABBGGRR.
      .color_rgba = request.color() == 0 ? UINT32_MAX : std::byteswap(request.color()),
      .glow = request.glow(),
      .solid = request.solid(),
  };
  auto model = (*effects)->CreateModel(settings);
  if (!model) {
    return std::unexpected(model.error());
  }
  return Keep(std::move(*model));
}

std::expected<ObjectResponse, std::string> Game::CreateText(const TextOptions& request) {
  // Resolve the text factory.
  auto text = services_.Text();
  if (!text) {
    return std::unexpected(text.error());
  }

  // Create the text and keep it as one of the mod's objects.
  render::WorldTextStyle style{
      .font_size = request.font_size() == 0 ? 100 : request.font_size(),
      // The entity takes its color as 0xAABBGGRR.
      .color_abgr = request.color() == 0 ? UINT32_MAX : std::byteswap(request.color()),
      .face_camera = request.face_camera(),
      .scale = request.scale() == 0 ? 1 : request.scale(),
  };
  auto entity = (*text)->Create(request.text(), request.position(), request.facing(), style);
  if (!entity) {
    return std::unexpected(entity.error());
  }
  return Keep(std::move(*entity));
}

std::expected<void, std::string> Game::MoveObject(const MoveObjectRequest& request) {
  auto found = objects_.find(request.object());
  if (found == objects_.end()) {
    return std::unexpected("no object has that identifier");
  }
  if (auto* model = std::get_if<std::unique_ptr<render::WorldEffect>>(&found->second)) {
    std::optional<std::array<float, 3>> angles;
    if (request.has_facing()) {
      angles = Floats(request.facing());
    }
    (*model)->Move(request.position(), angles);
  } else {
    std::get<std::unique_ptr<render::WorldTextEntity>>(found->second)
        ->SetOrigin(request.position(), request.facing());
  }
  return {};
}

std::expected<void, std::string> Game::SetText(const SetTextRequest& request) {
  // Find the object and check it is text.
  auto found = objects_.find(request.object());
  if (found == objects_.end()) {
    return std::unexpected("no object has that identifier");
  }
  auto* text = std::get_if<std::unique_ptr<render::WorldTextEntity>>(&found->second);
  if (text == nullptr) {
    return std::unexpected("the object is not text");
  }

  // Replace the words.
  (*text)->SetMessage(request.text());
  return {};
}

std::expected<void, std::string> Game::SetObjectHidden(const SetObjectHiddenRequest& request) {
  // Showing the object again drops it from the player's hidden set.
  if (!objects_.contains(request.object())) {
    return std::unexpected("no object has that identifier");
  }
  if (!request.hidden()) {
    // Unhide the object, forgetting an empty set.
    auto hidden = hidden_.find(request.player());
    if (hidden == hidden_.end()) {
      return {};
    }
    hidden->second.erase(request.object());
    if (hidden->second.empty()) {
      hidden_.erase(hidden);
    }
    return {};
  }

  // The engine sends only what its transmit pass allows, so hiding needs the
  // shared transmit hook.
  if (auto transmit = services_.Transmit(); !transmit) {
    return transmit;
  }
  hidden_[request.player()].insert(request.object());
  return {};
}

std::expected<void, std::string> Game::RemoveObject(const ObjectRequest& request) {
  // Find the object.
  auto found = objects_.find(request.object());
  if (found == objects_.end()) {
    return std::unexpected("no object has that identifier");
  }

  // Remove it from the world and from every hidden set.
  std::visit([](auto& handle) { handle->Remove(); }, found->second);
  objects_.erase(found);
  for (auto& [slot, objects] : hidden_) {
    objects.erase(request.object());
  }
  return {};
}

std::expected<EntityResponse, std::string> Game::ObjectEntity(const ObjectRequest& request) {
  // Find the object and check it is a model.
  auto found = objects_.find(request.object());
  if (found == objects_.end()) {
    return std::unexpected("no object has that identifier");
  }
  auto* model = std::get_if<std::unique_ptr<render::WorldEffect>>(&found->second);
  if (model == nullptr) {
    return std::unexpected("the object is text, which has no entity");
  }

  // Answer the model's live entity.
  auto handle = (*model)->Handle();
  if (!handle) {
    return std::unexpected("the object is no longer live");
  }
  EntityResponse response;
  response.set_entity(*handle);
  return response;
}

std::expected<BotResponse, std::string> Game::AddBot(const BotOptions& request) {
  // Resolve the hero and the bot service.
  auto heroes = services_.Heroes();
  if (!heroes) {
    return std::unexpected(heroes.error());
  }
  auto hero = FindHero(**heroes, request);
  if (!hero) {
    return std::unexpected(hero.error());
  }
  auto bots = services_.Bots();
  if (!bots) {
    return std::unexpected(bots.error());
  }

  // Create the bot and remember it, so Clear can kick it.
  auto slot = (*bots)->Create(request.name().c_str(), request.team(),
                              static_cast<uint32_t>(hero->id), Floats(request.position()));
  if (!slot) {
    return std::unexpected(slot.error());
  }
  bots_.emplace(*slot, request.name());
  BotResponse response;
  response.set_player(*slot);
  return response;
}

std::expected<void, std::string> Game::RemoveBot(const PlayerRequest& request) {
  // Find the bot the mod added in that slot.
  auto found = bots_.find(request.player());
  if (found == bots_.end()) {
    return std::unexpected("the mod added no bot in that slot");
  }

  // Kick the bot through the server console.
  auto server = services_.Server();
  if (!server) {
    return std::unexpected(server.error());
  }
  if (!(*server)->ServerCommand(("kick \"" + found->second + "\"\n").c_str())) {
    return std::unexpected("the server console is unavailable");
  }
  Depart(found->first);
  bots_.erase(found);
  return {};
}

std::expected<void, std::string> Game::Precache(const PrecacheOptions& request) {
  return engine_->Precache({request.heroes().begin(), request.heroes().end()},
                           {request.resources().begin(), request.resources().end()});
}

// The input calls replace the mod's changes; the shared hook applies them.

std::expected<void, std::string> Game::BlockInput(const InputRequest& request) {
  blocked_ = request.buttons();
  services_.Input();
  return {};
}

std::expected<void, std::string> Game::BlockPlayerInput(const PlayerInputRequest& request) {
  if (request.buttons() == 0) {
    slot_blocked_.erase(request.player());
  } else {
    slot_blocked_[request.player()] = request.buttons();
  }
  services_.Input();
  return {};
}

std::expected<void, std::string> Game::Press(const PlayerInputRequest& request) {
  presses_[request.player()] |= request.buttons();
  services_.Input();
  return {};
}

std::expected<void, std::string> Game::WatchInput(const InputRequest& request) {
  watched_ = request.buttons();
  services_.Input();
  return {};
}

std::expected<void, std::string> Game::RemapInput(const RemapInputRequest& request) {
  remap_ = request;
  services_.Input();
  return {};
}

gameinterop::WorldEntities::Prepare Game::WriteBeforeSpawn(
    const google::protobuf::RepeatedPtrField<FieldWrite>& fields) {
  // The entity is not networked yet, so its fields need no change notice.
  return [this, &fields](void* created) -> std::expected<void, std::string> {
    for (const auto& write : fields) {
      auto stored =
          StoreField(created, write.class_name(), write.field(), write.type(), write.value());
      if (!stored) {
        return stored;
      }
    }
    return {};
  };
}

std::expected<EntityResponse, std::string> Game::CreateEntity(const EntityOptions& request) {
  // Resolve the world and the requested key values.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  std::deque<std::string> filled;
  auto key_values = KeyValues(request.key_values(), addon_, filled);
  if (!key_values) {
    return std::unexpected(key_values.error());
  }

  // Describe the entity to create.
  const auto subclass =
      request.subclass().empty() ? 0 : gameinterop::WorldEntities::SubclassId(request.subclass());
  gameinterop::WorldEntities::Target target{
      .designer_name = request.designer_name(),
      .subclass_id = subclass,
      .team = request.team(),
      .position = Floats(request.position()),
      .facing = Floats(request.facing()),
      .velocity = {},
      .health = 0,
      .max_health = 0,
      .lane = std::nullopt,
  };

  // Create the entity, sent to every client when asked.
  auto entity = (*world)->CreateEntity(target, *key_values, WriteBeforeSpawn(request.fields()));
  if (!entity) {
    return std::unexpected(entity.error());
  }
  if (request.transmit_always()) {
    auto sent = (*world)->TransmitAlways(*entity);
    if (!sent || !*sent) {
      (void)(*world)->RemoveEntity(*entity);
      return std::unexpected(sent ? request.designer_name() + " did not survive its spawn"
                                  : sent.error());
    }
  }

  // Remember the entity as the mod's own and return it.
  created_.insert(*entity);
  EntityResponse response;
  response.set_entity(*entity);
  return response;
}

std::expected<AliveResponse, std::string> Game::FireInput(const FireInputRequest& request) {
  // Fire the input on the entity, from its activator when given.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  std::optional<uint32_t> activator;
  if (request.has_activator()) {
    activator = request.activator();
  }
  return Alive(
      (*world)->FireInput(request.entity(), request.input(), Value(request.value()), activator));
}

std::expected<AliveResponse, std::string> Game::RemoveEntity(const EntityRequest& request) {
  // Resolve the world.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }

  // Only the mod's own entities are removed, so a stale or mistaken handle
  // cannot take a player or the map's entities out of the world.
  if (!created_.erase(request.entity())) {
    return std::unexpected("the mod did not create the entity");
  }
  audiences_.erase(request.entity());
  return Alive((*world)->RemoveEntity(request.entity()));
}

std::expected<void, std::string> Game::ShowEntityOnlyTo(const ShowEntityOnlyToRequest& request) {
  // Only the mod's own entities take an audience, so a mod cannot keep a
  // player's hero or the map's entities from a client.
  if (!created_.contains(request.entity())) {
    return std::unexpected("the mod did not create the entity");
  }
  uint64_t slots = 0;
  for (const auto slot : request.players()) {
    if (!gameinterop::IsPlayerSlot(slot)) {
      return std::unexpected("player slot out of range");
    }
    slots |= uint64_t{1} << slot;
  }

  // The engine sends only what its transmit pass allows, so an audience
  // needs the shared transmit hook.
  if (auto transmit = services_.Transmit(); !transmit) {
    return transmit;
  }
  audiences_[request.entity()] = slots;
  return {};
}

std::expected<void, std::string> Game::ShowEntityToAll(const EntityRequest& request) {
  if (!created_.contains(request.entity())) {
    return std::unexpected("the mod did not create the entity");
  }
  audiences_.erase(request.entity());
  return {};
}

void Game::PruneAudiences() {
  if (audiences_.empty() || !world_) {
    return;
  }
  // The transmit pass withholds by entity index alone, so a gone entity's
  // audience would otherwise apply to the next entity at its index.
  std::erase_if(audiences_, [this](const auto& entry) {
    auto live = world_->Exists(entry.first);
    return live && !*live;
  });
}

std::expected<NpcResponse, std::string> Game::SpawnNpc(const NpcOptions& request) {
  // Resolve the world.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }

  // Describe the NPC, defaulting its maximum health to its health.
  const int32_t max_health = request.max_health() == 0 ? request.health() : request.max_health();
  gameinterop::WorldEntities::Target target{
      .designer_name = request.class_name(),
      .subclass_id =
          request.unit().empty() ? 0 : gameinterop::WorldEntities::SubclassId(request.unit()),
      .team = request.team(),
      .position = Floats(request.position()),
      .facing = Floats(request.facing()),
      .velocity = {},
      .health = request.health(),
      .max_health = max_health,
      .lane = request.has_lane() ? std::optional(request.lane()) : std::nullopt,
  };

  // Spawn the NPC and remember it as the mod's own.
  auto entity = (*world)->Spawn(target, WriteBeforeSpawn(request.fields()));
  if (!entity) {
    return std::unexpected(entity.error());
  }
  created_.insert(*entity);
  spawned_ = true;
  NpcResponse response;
  response.set_npc(*entity);
  return response;
}

std::expected<NpcStateResponse, std::string> Game::ReadNpc(const NpcRequest& request) {
  // Read the NPC's state.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  auto sample = (*world)->ReadNpc(request.npc());
  if (!sample) {
    return std::unexpected(sample.error());
  }

  // An NPC that is gone reads as an empty state.
  NpcStateResponse response;
  if (!*sample) {
    return response;
  }

  // Copy the NPC's state into the response.
  const auto& state = (*sample)->state;
  auto* npc = response.mutable_state();
  SetVector(npc->mutable_position(), state.position);
  SetAngles(npc->mutable_facing(), state.facing);
  npc->set_health(state.health);
  npc->set_max_health(state.max_health);
  npc->set_team(state.team);
  return response;
}

std::expected<AliveResponse, std::string> Game::MoveNpc(const MoveNpcRequest& request) {
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  return Alive((*world)->Move(request.npc(), Floats(request.position()), Floats(request.facing()),
                              Floats(request.velocity())));
}

std::expected<AliveResponse, std::string> Game::SetNpcHealth(const SetNpcHealthRequest& request) {
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  const int32_t max_health = request.has_max_health() ? request.max_health() : request.health();
  return Alive((*world)->SetHealth(request.npc(), request.health(), max_health));
}

std::expected<AliveResponse, std::string> Game::RemoveNpc(const NpcRequest& request) {
  EntityRequest entity;
  entity.set_entity(request.npc());
  return RemoveEntity(entity);
}

std::expected<PickupResponse, std::string> Game::CreatePickup(const CreatePickupRequest& request) {
  // Resolve the world.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }

  // A pickup reads its aura from its game data entry. The urn's entry shares
  // its designer name; a movement buff names the movement pickup entry.
  const bool urn = request.kind() == PICKUP_KIND_URN;
  gameinterop::WorldEntities::Target target{
      .designer_name = urn ? "citadel_item_pickup_idol" : "citadel_item_pickup",
      .subclass_id = gameinterop::WorldEntities::SubclassId(urn ? "citadel_item_pickup_idol"
                                                                : "movement_powerup_pickup"),
      .team = 0,
      .position = Floats(request.position()),
      .facing = {},
      .velocity = {},
      .health = 0,
      .max_health = 0,
      .lane = std::nullopt,
  };

  // Create the pickup and remember it as the mod's own.
  auto entity = (*world)->CreateEntity(target, {});
  if (!entity) {
    return std::unexpected(entity.error());
  }
  created_.insert(*entity);
  PickupResponse response;
  response.set_pickup(*entity);
  return response;
}

std::expected<AliveResponse, std::string> Game::PickupPresent(const PickupRequest& request) {
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  return Alive((*world)->Exists(request.pickup()));
}

std::expected<AliveResponse, std::string> Game::RemovePickup(const PickupRequest& request) {
  EntityRequest entity;
  entity.set_entity(request.pickup());
  return RemoveEntity(entity);
}

std::expected<EntitiesResponse, std::string> Game::FindEntities(
    const FindEntitiesRequest& request) {
  // Find the entities of the class.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  auto found = (*world)->Find(request.class_name());
  if (!found) {
    return std::unexpected(found.error());
  }
  EntitiesResponse response;
  response.mutable_entities()->Add(found->begin(), found->end());
  return response;
}

std::expected<CountResponse, std::string> Game::RemoveEntities(
    const RemoveEntitiesRequest& request) {
  // Remove the world's entities of the class and count them.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  auto removed = (*world)->Remove(request.class_name());
  if (!removed) {
    return std::unexpected(removed.error());
  }
  CountResponse response;
  response.set_count(static_cast<int32_t>(*removed));
  return response;
}

std::expected<void, std::string> Game::ClearMap() {
  // Remove everything the mod authored in the world.
  auto world = World();
  if (!world) {
    return std::unexpected(world.error());
  }
  auto damage = services_.Damage();
  if (!damage) {
    return std::unexpected(damage.error());
  }
  return (*world)->ClearAuthored(**damage);
}

}  // namespace modlock::wasm

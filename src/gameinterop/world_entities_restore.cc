#include <cmath>
#include <optional>
#include <vector>

#include "modlock/gameinterop/entity_abi.h"
#include "modlock/gameinterop/native_damage.h"
#include "modlock/gameinterop/world_entities.h"
#include "world_entities_internal.h"

namespace modlock::gameinterop {

namespace {

// kMaxTeam is the highest team number a target may name.
constexpr int32_t kMaxTeam = 4;

// kRetainDistance is how far a live structure may sit from a matching
// target and still be kept in place of a new one.
constexpr float kRetainDistance = 64;

// DistanceSquared returns the squared distance between two points.
float DistanceSquared(const std::array<float, 3>& a, const std::array<float, 3>& b) {
  float result = 0;
  for (size_t i = 0; i < a.size(); ++i) {
    result += (a[i] - b[i]) * (a[i] - b[i]);
  }
  return result;
}

}  // namespace

std::expected<void, std::string> WorldEntities::Restore(std::span<const Target> targets) {
  // Forget the last restore.
  pending_.clear();

  // Validate every target and definition before removing any existing world state.
  for (const auto& target : targets) {
    if (!IsRestoredNpc(target.designer_name) || !target.subclass_id || target.health <= 0 ||
        target.max_health <= 0 || target.team < 0 || target.team > kMaxTeam ||
        !calls_.definition(-1, target.subclass_id)) {
      return std::unexpected("unsupported NPC target: " + target.designer_name);
    }
    for (const auto& vector : {target.position, target.facing, target.velocity}) {
      for (float value : vector) {
        if (!std::isfinite(value)) {
          return std::unexpected("NPC target motion is nonfinite");
        }
      }
    }
  }

  // Remove every restored NPC but a living structure that matches a target.
  auto system = calls_.entity_system();
  if (!system) {
    return std::unexpected(system.error());
  }
  std::vector<std::optional<uint32_t>> retained(targets.size());
  for (void* entity : EntityInstances(*system)) {
    // Keep a live structure that matches an unclaimed target; remove the rest.
    const auto name = DesignerName(entity);
    if (!IsRestoredNpc(name)) {
      continue;
    }
    auto sample = ReadEntity(entity, std::string(name));
    if (!sample) {
      return std::unexpected(sample.error());
    }
    bool keep = false;
    if (IsStructure(name) && sample->state.health > 0) {
      for (size_t i = 0; i < targets.size(); ++i) {
        const auto& target = targets[i];
        if (!retained[i] && target.designer_name == name && target.team == sample->state.team &&
            target.subclass_id == sample->state.subclass_id &&
            DistanceSquared(target.position, sample->state.position) <
                kRetainDistance * kRetainDistance) {
          retained[i] = sample->handle;
          keep = true;
          break;
        }
      }
    }
    if (!keep) {
      calls_.remove(entity);
    }
  }

  // Create the missing targets and place every one.
  for (size_t i = 0; i < targets.size(); ++i) {
    // Create each target not kept, apply its state, and wait for it.
    void* entity = retained[i] ? EntityInstance(*system, *retained[i]) : nullptr;
    if (!entity) {
      auto created = Create(targets[i]);
      if (!created) {
        return std::unexpected(created.error());
      }
      entity = *created;
    }
    auto applied = Apply(entity, targets[i]);
    if (!applied) {
      return std::unexpected(applied.error());
    }
    auto handle = ReferenceHandleOf(entity);
    if (!handle) {
      return std::unexpected("restored NPC identity is absent");
    }
    pending_.push_back({*handle, targets[i]});
  }
  return {};
}

std::expected<void, std::string> WorldEntities::FinishRestore() {
  // Place each pending NPC again after its initialization.
  auto system = calls_.entity_system();
  if (!system) {
    return std::unexpected(system.error());
  }
  for (const auto& sample : pending_) {
    void* entity = EntityInstance(*system, sample.handle);
    if (!entity) {
      return std::unexpected("restored NPC did not survive initialization");
    }
    auto applied = Apply(entity, sample.state);
    if (!applied) {
      return std::unexpected(applied.error());
    }
  }
  pending_.clear();
  return {};
}

std::expected<void, std::string> WorldEntities::ClearAuthored(const NativeDamage& damage) {
  // Collect the authored actors.
  auto system = calls_.entity_system();
  if (!system) {
    return std::unexpected(system.error());
  }
  std::vector<uint32_t> actors;
  for (void* entity : EntityInstances(*system)) {
    const auto name = DesignerName(entity);
    if (name != "npc_trooper_boss" && name != "npc_boss_tier2" &&
        !(IsRestoredNpc(name) && !IsStructure(name)) && !IsRoundObjective(name)) {
      continue;
    }
    if (const auto handle = ReferenceHandleOf(entity)) {
      actors.push_back(*handle);
    }
  }

  // Native death can remove other actors. Reborrow each serial-fenced identity
  // after every mutation instead of retaining the original instance pointers.
  for (const auto handle : actors) {
    // Remove round objectives and kill the other actors.
    void* entity = EntityInstance(*system, handle);
    if (!entity) {
      continue;
    }
    if (IsRoundObjective(DesignerName(entity))) {
      calls_.remove(entity);
      continue;
    }
    auto killed = damage.Kill(entity);
    if (!killed) {
      return killed;
    }
  }
  return {};
}

}  // namespace modlock::gameinterop

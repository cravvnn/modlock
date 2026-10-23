#pragma once

#include <array>
#include <cstring>
#include <expected>
#include <string>
#include <string_view>

#include "modlock/gameinterop/entity_abi.h"
#include "modlock/gameinterop/hero_definitions.h"
#include "src/wasm/wasm.pb.h"

// Helpers the translation units of Game and GameServices share.
namespace modlock::wasm {

// kMaxChanges bounds the button changes, and separately the launches, one
// mod has not yet received.
inline constexpr size_t kMaxChanges = 256;

// kDesignerNameOffset is CEntityIdentity's designer name pointer.
inline constexpr size_t kDesignerNameOffset = 0x20;

inline std::array<float, 3> Floats(const geometry::Vec3& vector) {
  return {static_cast<float>(vector.x()), static_cast<float>(vector.y()),
          static_cast<float>(vector.z())};
}

inline std::array<float, 3> Floats(const geometry::EulerAngles& angles) {
  return {static_cast<float>(angles.pitch()), static_cast<float>(angles.yaw()),
          static_cast<float>(angles.roll())};
}

inline void SetAngles(geometry::EulerAngles* out, const std::array<float, 3>& angles) {
  out->set_pitch(angles[0]);
  out->set_yaw(angles[1]);
  out->set_roll(angles[2]);
}

inline void SetVector(geometry::Vec3* out, const std::array<float, 3>& vector) {
  out->set_x(vector[0]);
  out->set_y(vector[1]);
  out->set_z(vector[2]);
}

// DesignerName returns an entity's designer name, such as player, or an
// empty view.
inline std::string_view DesignerName(void* entity) {
  // Find the entity's identity, which holds its names.
  const auto* identity = static_cast<const unsigned char*>(gameinterop::IdentityOf(entity));
  if (identity == nullptr) {
    return {};
  }

  // Read the designer name pointer out of the identity.
  const char* name = nullptr;
  std::memcpy(&name, identity + kDesignerNameOffset, sizeof(name));
  return name == nullptr ? std::string_view() : std::string_view(name);
}

// FindHero resolves a request's hero by its identifier or its name.
template <typename Request>
std::expected<gameinterop::HeroDefinition, std::string> FindHero(
    const gameinterop::HeroDefinitions& heroes, const Request& request) {
  if (request.hero_case() == Request::kHeroId) {
    return heroes.Find(request.hero_id());
  }
  return heroes.Find(request.hero_name());
}

}  // namespace modlock::wasm

#pragma once

#include <array>
#include <cstdint>
#include <expected>

#include "modlock/export.h"
#include "modlock/render/world_effect_seam.h"

namespace modlock::render {

// WorldParticle extends the shared effect lifetime with native particle inputs.
// All operations run on the engine frame thread. Remove and world invalidation
// make subsequent mutations unavailable; neither retains a borrowed pawn.
class MODLOCK_API WorldParticle : public WorldEffect {
 public:
  virtual std::expected<void, std::string> Start() = 0;
  virtual std::expected<void, std::string> Stop() = 0;
  virtual std::expected<void, std::string> Transform(const geometry::Vec3& origin,
                                                     const std::array<float, 3>& angles) = 0;
  virtual std::expected<void, std::string> Tint(const ParticleTint& tint) = 0;
  virtual std::expected<void, std::string> Data(const ParticleData& data) = 0;
  virtual std::expected<void, std::string> ControlPoint(int index, std::uint32_t entity_handle) = 0;
  virtual std::expected<void, std::string> Attach(std::uint32_t parent_handle) = 0;
  virtual std::expected<void, std::string> Detach() = 0;
};

}  // namespace modlock::render

#pragma once

#include <cstdint>
#include <expected>
#include <memory>
#include <optional>
#include <span>
#include <string>
#include <utility>
#include <vector>

#include "modlock/export.h"
#include "modlock/gameinterop/pawn_observer.h"

namespace modlock::gameinterop {

// StaminaEvidenceTracker owns the expenditure counter for one pawn incarnation.
// Bind and Record run on one engine frame thread. Evidence is absent until an
// attributed positive spend proves liveness. After a gap, a new positive spend
// starts a new epoch; the interrupted attempt cannot reuse its old baseline.
class MODLOCK_API StaminaEvidenceTracker {
 public:
  std::optional<modlock::StaminaEvidence> Bind(const std::optional<PawnObserver::Sample>& sample);
  // Record accepts the engine serializer's varint-size-delimited protobuf.
  // It never accepts a raw payload or infers expenditure from regeneration.
  void Record(std::span<const uint8_t> serialized);

  // RecordDamage accepts one engine-framed Damage message. It credits positive
  // head damage only when the bound pawn is the attributed attacker.
  void RecordDamage(std::span<const uint8_t> serialized);

  // ConsumeHeadshots returns and clears this frame's distinct credited hits.
  [[nodiscard]] uint32_t ConsumeHeadshots();
  void Invalidate();

 private:
  void StartEpoch();
  void ResetHeadshots();

  struct Identity {
    uint32_t pawn_handle;
    uint64_t steam_id;
    uint32_t generation;
    bool operator==(const Identity&) const = default;
  };

  std::optional<Identity> identity_;
  uint64_t epoch_ = 0;
  double spent_ = 0;
  bool live_ = false;
  bool lost_ = false;
  std::optional<float> last_time_;
  std::optional<std::pair<float, float>> last_values_;

  struct HeadshotKey {
    int32_t attacker;
    int32_t victim;
    int32_t server_tick;
    bool operator==(const HeadshotKey&) const = default;
  };

  std::vector<HeadshotKey> recent_headshots_;
  uint32_t pending_headshots_ = 0;
};

// StaminaObserver owns the native event hook and serializes callbacks with pawn
// sampling. Install runs before engine handoff; destruction follows the engine
// return. Only game-serialized bytes cross the protobuf ABI boundary.
class MODLOCK_API StaminaObserver {
 public:
  static std::expected<StaminaObserver, std::string> Install();
  StaminaObserver(StaminaObserver&&) noexcept;
  StaminaObserver& operator=(StaminaObserver&&) noexcept;
  ~StaminaObserver();
  void Observe(std::optional<PawnObserver::Sample>& sample);

 private:
  struct Impl;
  explicit StaminaObserver(std::unique_ptr<Impl> impl);
  std::unique_ptr<Impl> impl_;
};

}  // namespace modlock::gameinterop

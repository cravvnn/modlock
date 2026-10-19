#pragma once

#include <array>
#include <cstdint>
#include <expected>
#include <functional>
#include <memory>
#include <string>

#include "modlock/export.h"
#include "modlock/gameinterop/game_symbols.h"

namespace modlock::gameinterop {

// BotLeash decides each game-driven bot's retreat. The bot brain asks its
// retreat check before any other goal but a pending ability, whenever it picks
// a schedule; the check walks a hurt bot home to heal. The leash answers that
// check for the bot's hero pawn: as the game would, never, or by walking the
// bot to a point the session chooses. Install before play and retain until the
// engine stops. The callback runs on the engine thread and must outlive the
// leash; only one leash may be installed.
class MODLOCK_API BotLeash {
 public:
  // Retreat is the answer for one bot.
  struct Retreat {
    enum class Kind : uint8_t {
      // kNative runs the game's own check.
      kNative,
      // kNever reports no retreat, so the bot keeps fighting when hurt.
      kNever,
      // kWalkTo walks the bot toward goal, ahead of every fight.
      kWalkTo,
    };
    Kind kind = Kind::kNative;
    std::array<float, 3> goal{};
  };

  // Decide answers for the bot whose hero pawn has the handle.
  using Decide = std::function<Retreat(uint32_t)>;

  static std::expected<BotLeash, std::string> Install(const ModuleImage& server, Decide decide);
  BotLeash(BotLeash&&) noexcept;
  BotLeash& operator=(BotLeash&&) noexcept;
  ~BotLeash();

 private:
  struct Impl;
  explicit BotLeash(std::unique_ptr<Impl> impl);
  std::unique_ptr<Impl> impl_;
};

}  // namespace modlock::gameinterop

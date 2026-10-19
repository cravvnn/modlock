#pragma once

#include <array>
#include <cstdint>
#include <expected>
#include <functional>
#include <memory>
#include <optional>
#include <string>

#include "modlock/export.h"
#include "modlock/gameinterop/game_symbols.h"

namespace modlock::gameinterop {

// BotDrive replaces a bot's input with a session's. Every hero's commands
// reach movement through CBasePlayerController::ProcessUsercmds, a bot's as
// one command its brain built this tick, so a command rewritten there runs as
// a player's own: movement, movement abilities, casts and animation all follow
// it. Install before play and retain until the engine stops. The callback runs
// on the engine thread and must outlive the drive; only one may be installed.
class MODLOCK_API BotDrive {
 public:
  // Command is a player's controls for one command.
  struct Command {
    // Angles are pitch, yaw and roll in degrees.
    std::array<float, 3> angles{};
    // Forward and Left are the movement axes, from -1 to 1.
    float forward = 0;
    float left = 0;
    // Held are the buttons down; Pressed went down in this command, even when
    // already held.
    uint64_t held = 0;
    uint64_t pressed = 0;
  };

  // Decide returns the command for the controller in slot, or nothing to
  // leave its own.
  using Decide = std::function<std::optional<Command>(int32_t slot)>;

  static std::expected<BotDrive, std::string> Install(const ModuleImage& server, Decide decide);
  BotDrive(BotDrive&&) noexcept;
  BotDrive& operator=(BotDrive&&) noexcept;
  ~BotDrive();

 private:
  struct Impl;
  explicit BotDrive(std::unique_ptr<Impl> impl);
  std::unique_ptr<Impl> impl_;
};

}  // namespace modlock::gameinterop

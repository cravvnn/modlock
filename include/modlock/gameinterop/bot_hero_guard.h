#pragma once

#include <expected>
#include <memory>
#include <string>

#include "modlock/export.h"
#include "modlock/gameinterop/game_symbols.h"

namespace modlock::gameinterop {

// BotHeroGuard makes the stock bot brain honor citadel_allow_duplicate_heroes.
// In a solo bot match the brain checks its hero about once a second and clears
// it when a human plays the same one, whatever that setting says, so a bot
// could not mirror a human's hero in a mode whose bots fight. With the guard
// installed and the setting on, the check finds no human and the bot keeps
// its hero; with the setting off the brain behaves as stock. Install on the
// engine thread before play and retain until the engine stops; only one guard
// may be installed.
class MODLOCK_API BotHeroGuard {
 public:
  static std::expected<BotHeroGuard, std::string> Install(const ModuleImage& server);
  BotHeroGuard(BotHeroGuard&&) noexcept;
  BotHeroGuard& operator=(BotHeroGuard&&) noexcept;
  ~BotHeroGuard();

 private:
  struct Impl;
  explicit BotHeroGuard(std::unique_ptr<Impl> impl);
  std::unique_ptr<Impl> impl_;
};

}  // namespace modlock::gameinterop

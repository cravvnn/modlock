#pragma once

#include <cstdint>
#include <expected>
#include <functional>
#include <memory>
#include <string>

#include "modlock/export.h"

namespace modlock::gameinterop {

// UserMessageFilter keeps server user messages from chosen players. It wraps
// the engine's PostEventAbstract, asks for each message which player slots must
// not receive it, and sends the message to the rest, or to nobody when none
// remain. Messages posted off the installing engine thread pass unchanged.
// Destruction restores dispatch; a hook stacked on the same slot after this one
// keeps working either way, since the thunk always forwards.
class MODLOCK_API UserMessageFilter {
 public:
  // Withheld returns the mask of player slots that must not receive the
  // message with the given network ID.
  using Withheld = std::function<uint64_t(int32_t)>;

  // Install borrows GameEventSystemServerV001 after world initialization.
  static std::expected<UserMessageFilter, std::string> Install(Withheld withheld);

  UserMessageFilter(UserMessageFilter&&) noexcept;
  UserMessageFilter& operator=(UserMessageFilter&&) noexcept;
  ~UserMessageFilter();

 private:
  struct Impl;
  explicit UserMessageFilter(std::unique_ptr<Impl> impl);
  std::unique_ptr<Impl> impl_;
};

}  // namespace modlock::gameinterop

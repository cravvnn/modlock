#pragma once

#include <cstdint>
#include <expected>
#include <functional>
#include <memory>
#include <optional>
#include <span>
#include <string>
#include <string_view>

#include "modlock/export.h"

namespace modlock::gameinterop {

// NativeChatHook observes server-accepted global and team chat on the installing
// engine thread. It never suppresses chat or changes its recipients. Destruction
// restores dispatch after the engine thread has quiesced.
class MODLOCK_API NativeChatHook {
 public:
  using Handler = std::function<void(int32_t, std::string_view)>;

  struct Message {
    int32_t slot;
    std::string text;
  };

  // Decode accepts native length-prefixed human chat bytes. The live hook
  // separately verifies the decoded slot's current authenticated connection.
  static std::optional<Message> Decode(int16_t message_id, std::span<const uint8_t> serialized);

  // Install borrows GameEventSystemServerV001 after world initialization.
  static std::expected<NativeChatHook, std::string> Install(Handler handler);
  NativeChatHook(NativeChatHook&&) noexcept;
  NativeChatHook& operator=(NativeChatHook&&) noexcept;
  ~NativeChatHook();

 private:
  struct Impl;
  explicit NativeChatHook(std::unique_ptr<Impl> impl);
  std::unique_ptr<Impl> impl_;
};

}  // namespace modlock::gameinterop

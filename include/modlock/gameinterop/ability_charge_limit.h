#pragma once

#include <cstddef>
#include <cstdint>
#include <expected>
#include <memory>
#include <optional>
#include <string>

#include "modlock/export.h"

namespace modlock::gameinterop {

// AbilityChargeLimit replaces the most charges chosen abilities can hold. It
// hooks the native GetMaxCharges entry of an ability's dispatch table and
// answers a limit for the abilities that have one by entity handle; every
// other ability, and every other query, reaches the game's own answer. The
// handle carries the entity's serial, so a limit never reaches a different
// ability created at the same address. The server holds the maximum, so the
// client's own display of it is not changed.
// Calls, installation and destruction belong to the engine thread. Only one
// AbilityChargeLimit lives at a time. Destruction restores every entry.
class MODLOCK_API AbilityChargeLimit {
 public:
  // HandleOf reads an ability's entity handle, or nothing when it is leaving.
  using HandleOf = std::optional<uint32_t> (*)(void* ability);

  // kMostCharges bounds a limit, so a bad value cannot make an ability endless.
  static constexpr int32_t kMostCharges = 100;

  // Create hooks nothing yet. slot is the GetMaxCharges index of the server's
  // ability dispatch table, from ResolveAbilityChargeSlots. handle_of reads an
  // ability's handle; the default is ReferenceHandleOf.
  static std::expected<std::unique_ptr<AbilityChargeLimit>, std::string> Create(
      size_t slot, HandleOf handle_of = nullptr);

  ~AbilityChargeLimit();
  AbilityChargeLimit(const AbilityChargeLimit&) = delete;
  AbilityChargeLimit& operator=(const AbilityChargeLimit&) = delete;

  // Set makes maximum the most charges the ability holds, hooking the
  // ability's dispatch table the first time one of its class is limited. A
  // maximum of 0 returns the ability to the game's own. The ability's handle
  // must be the one handle_of reads from it.
  std::expected<void, std::string> Set(void* ability, uint32_t handle, int32_t maximum);

  // Limit reports the maximum Set gave the handle, or nothing.
  [[nodiscard]] std::optional<int32_t> Limit(uint32_t handle) const;

  // Impl holds the hooks and limits; the replacement entry reads it.
  struct Impl;

 private:
  explicit AbilityChargeLimit(std::unique_ptr<Impl> impl);
  std::unique_ptr<Impl> impl_;
};

}  // namespace modlock::gameinterop

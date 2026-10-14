#pragma once

#include <cstddef>
#include <cstdint>
#include <cstring>

#include "modlock/gameinterop/virtual_slots.h"

namespace modlock::gameinterop {

// Since game build 6711 a message serializer is a plain record rather than a
// virtual class: its allocator constructs messages, the message's deleting
// destructor frees them, and INetworkMessages serializes and parses them.
// Source SDK networksystem/inetworkserializer.h.
inline constexpr size_t kSerializerAllocateOffset = 0x8;
inline constexpr size_t kSerializerMessageIdOffset = 0x18;

// NetworkMessageIdOf reads the 32-bit message ID from a native serializer.
inline int32_t NetworkMessageIdOf(const void* serializer) {
  int32_t id = 0;
  std::memcpy(&id, static_cast<const std::byte*>(serializer) + kSerializerMessageIdOffset,
              sizeof(id));
  return id;
}

// NetworkMessagesMethod returns one INetworkMessages virtual method.
template <typename Method>
Method NetworkMessagesMethod(void* messages, size_t slot) {
  return reinterpret_cast<Method>((*static_cast<void***>(messages))[slot]);
}

}  // namespace modlock::gameinterop

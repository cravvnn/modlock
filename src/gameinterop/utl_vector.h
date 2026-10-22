#pragma once

#include <cstdint>
#include <cstring>
#include <optional>

namespace modlock::gameinterop {

// UtlVector is the game's x64 CUtlVector<T> as it lies in memory: an int
// count, alignment padding, then CUtlVectorMemory's data pointer, allocation
// count and growth flags.
template <typename T>
struct UtlVector {
  int32_t count;
  uint32_t padding;
  T* data;
  uint32_t capacity;
  uint32_t flags;
};

static_assert(sizeof(UtlVector<uint32_t>) == 24);

// kMaxAbilities and kMaxModifiers bound the ability and modifier vectors a
// hero carries; a larger count means the read caught torn memory.
inline constexpr int32_t kMaxAbilities = 256;
inline constexpr int32_t kMaxModifiers = 1024;

// ReadUtlVector copies the vector at address and checks its bounds: a count
// from zero to max_count within its capacity, and data when it has any. A
// torn or freed vector reads as nullopt.
template <typename T>
std::optional<UtlVector<T>> ReadUtlVector(const void* address, int32_t max_count) {
  UtlVector<T> vector{};
  std::memcpy(&vector, address, sizeof(vector));
  if (vector.count < 0 || vector.count > max_count ||
      static_cast<uint32_t>(vector.count) > vector.capacity || (vector.count && !vector.data)) {
    return std::nullopt;
  }
  return vector;
}

}  // namespace modlock::gameinterop

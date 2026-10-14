#pragma once

#include <cstddef>
#include <functional>

#include "modlock/export.h"

namespace modlock::gameinterop {

// BoundedReader and BoundedWriter reject inaccessible native memory instead of
// dereferencing it. Success means the entire requested region was copied.
using BoundedReader = std::function<bool(const void* source, void* out, size_t size)>;
using BoundedWriter = std::function<bool(void* target, const void* source, size_t size)>;

// ReadNative copies size bytes from source through the operating system, so
// an unmapped or protected address fails instead of faulting. It always fails
// off Windows.
MODLOCK_API bool ReadNative(const void* source, void* out, size_t size);

// WriteNative copies size bytes to target the same way ReadNative reads.
MODLOCK_API bool WriteNative(void* target, const void* source, size_t size);

// AllocateNative returns size bytes from the game's allocator, or null when it
// has none, so the game can free what it receives, as with a list field it
// grows. It always returns null off Windows.
MODLOCK_API void* AllocateNative(size_t size);

}  // namespace modlock::gameinterop

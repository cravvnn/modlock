#pragma once

#include <cstdint>
#include <expected>
#include <string>

#include "modlock/export.h"
#include "modlock/gameinterop/game_symbols.h"

namespace modlock::gameinterop {

// Resolves replay subclass IDs through the engine's current ability registry.
// The mapped server module owns the function and returned definition storage;
// callers discard borrowed definitions on world shutdown.
class MODLOCK_API AbilityDefinitions {
 public:
  using Lookup = void* (*)(int32_t scope, uint32_t subclass_id);

  explicit AbilityDefinitions(Lookup lookup) : lookup_(lookup) {}

  static std::expected<AbilityDefinitions, std::string> Resolve(const ModuleImage& server);

  struct Definition {
    std::string name;
    bool disabled;
    // native_definition_pointer is borrowed from the mapped engine registry.
    void* native_definition_pointer;
  };

  // Missing, disabled and enabled definitions remain distinct. This lookup
  // grants neither ownership nor permission to claim an item was restored.
  std::expected<Definition, std::string> Find(uint32_t subclass_id) const;

 private:
  Lookup lookup_;
};

}  // namespace modlock::gameinterop

#pragma once

#include <cstddef>
#include <cstdint>
#include <expected>
#include <string>

#include "modlock/export.h"

namespace modlock::gameinterop {

// EngineToServerVersion is engine2.dll's IVEngineServer2 interface version
// (sourcesdk public/interfaces/interfaces.h:
// SOURCE2ENGINETOSERVER_INTERFACE_VERSION).
inline constexpr char kEngineToServerVersion[] = "Source2EngineToServer001";

// Vtable slot of IVEngineServer2::ServerCommand. Counted from the exact
// sourcesdk declaration order: IAppSystem's eleven slots, ISource2Engine's
// seven (IsPaused through UnknownFunc2), then the twenty-seven IVEngineServer2
// virtuals before ServerCommand (GetSteamUniverse through
// Message_DetermineMulticastRecipients, including GetLongFrameCount since
// game build 6711).
inline constexpr size_t kServerCommandSlot = 45;
// Same SDK declaration: GetServerGlobals follows IsClientFullyAuthenticated.
// Game build 6779 added one virtual between ServerCommand and
// IsClientFullyAuthenticated, which moved this slot from 75 to 76.
inline constexpr size_t kServerGlobalsSlot = 76;

// EngineServer is a resolved live IVEngineServer2 instance. It issues console
// commands through the engine's own parser - identical to typing them at the
// server console - so automated setup rides exactly the path an operator would.
class MODLOCK_API EngineServer {
 public:
  // Resolve finds the mapped engine2.dll, requests the interface, and checks
  // that its dispatch table carries the ServerCommand slot. Failure names the
  // missing piece.
  static std::expected<EngineServer, std::string> Resolve();
  // Binds the factory's borrowed Source2EngineToServer001 instance. The engine
  // owns it; callers discard this view before unloading engine2.dll.
  static std::expected<EngineServer, std::string> Bind(void* instance);

  struct SimulationClock {
    float current_time;
    int32_t tick;
    float interval;
  };
  // Reads GetServerGlobals during the engine frame. Returned values are copied;
  // no globals pointer survives the call. Pauses follow simulation time.
  [[nodiscard]] std::expected<SimulationClock, std::string> ReadClock() const;

  // ServerCommand issues one command line through the engine's command parser
  // (IVEngineServer2::ServerCommand, sourcesdk public/eiface.h).
  // Returns false when the interface or command slot is unavailable.
  [[nodiscard]] bool ServerCommand(const char* command) const;

 private:
  void* instance_ = nullptr;
};

}  // namespace modlock::gameinterop

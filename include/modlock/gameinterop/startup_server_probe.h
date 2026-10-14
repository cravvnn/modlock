#pragma once

#include <cstddef>
#include <expected>
#include <functional>
#include <string>
#include <string_view>

#include "modlock/export.h"
#include "modlock/gameinterop/frame_hook.h"

namespace modlock::gameinterop {

// Engine2's network-server-service interface version string.
inline constexpr char kNetworkServerServiceVersion[] = "NetworkServerService_001";

// StartupServerProbe joins one observer to the engine's own server-session
// startup: every INetworkServerService::StartupServer call invokes the
// before callback, runs the engine original, then invokes the after callback
// on the engine thread before any player can be admitted. This is the one
// lifecycle point where the convar registry is fully populated, even though
// a hibernating dedicated host may
// never tick GameFrame before a client joins (live evidence 2026-08-25: the
// first-frame trigger never fired pre-hibernation).
//
// The probe patches its own dispatch-table entry through VtableSlotHook, so it
// coexists with the GameFrame entry and shares no thunk state with it; the
// move and destruction ordering is ThunkOwner's contract.
class MODLOCK_API StartupServerProbe {
 public:
  // Startup callbacks run on the engine thread around the engine's own
  // StartupServer work: before_startup immediately before the original, then
  // after_startup after it returns and before player admission proceeds. The
  // map_name view is borrowed and must not be retained by the probe.
  using StartupCallback = std::function<void(std::string_view map_name)>;

  // DispatchCallbacks applies the startup lifecycle order used by the engine
  // thunk. map_name is borrowed from the engine and is valid only during this
  // call; consumers copy it when retaining world identity.
  static void DispatchCallbacks(std::string_view map_name, const StartupCallback& before_startup,
                                const StartupCallback& original,
                                const StartupCallback& after_startup);

  // Install resolves engine2.dll's NetworkServerService_001 interface and
  // patches its StartupServer slot. Failure names the missing piece (module,
  // export, interface).
  static std::expected<StartupServerProbe, std::string> Install(
      StartupCallback before_startup, StartupCallback after_startup,
      std::function<void()> before_shutdown = {}, std::function<void(std::string)> on_error = {});

  StartupServerProbe(StartupServerProbe&& other) noexcept = default;
  StartupServerProbe& operator=(StartupServerProbe&& other) noexcept = default;
  ~StartupServerProbe() = default;

 private:
  // Private by construction: only Install builds one, around a live slot hook.
  explicit StartupServerProbe(ThunkOwner owner);

  ThunkOwner owner_;
};

}  // namespace modlock::gameinterop

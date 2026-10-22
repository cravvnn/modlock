#include "modlock/gameinterop/bot_drive.h"

#include <cstddef>
#include <cstdio>
#include <cstring>
#include <utility>

#include "modlock/gameinterop/connection_tracker.h"
#include "modlock/gameinterop/entity_abi.h"

#if defined(_WIN32)
#include <safetyhook.hpp>
#endif

namespace modlock::gameinterop {
namespace {

#if defined(_WIN32)
// CUserCmd has no schema. Build 6795: CCitadelPlayerBot::PhysicsSimulate
// (server RVA 0x817cf0) and the test-convar writer at RVA 0x7e2700 build a
// bot's command with these, and the field getters at RVA 0x7e90e0 and
// 0x7e9100 read the axes back.
constexpr size_t kCommandStride = 0xa8;
// The CCitadelUserCmdPB begins at +0x10; its has-bits word, whose bit 0 marks
// the base message, is at +0x20, and its base message pointer at +0x48.
constexpr size_t kCommandHasBits = 0x20;
constexpr size_t kCommandBase = 0x48;
// CBaseUserCmdPB: has-bits, the view angles message, forwardmove (has-bit
// 0x40) and leftmove (has-bit 0x80).
constexpr size_t kBaseHasBits = 0x10;
constexpr size_t kBaseAngles = 0x40;
constexpr size_t kBaseForward = 0x58;
constexpr size_t kBaseLeft = 0x5c;
constexpr uint32_t kBaseAxesBits = 0x40 | 0x80;
// CMsgQAngle holds x, y and z from +0x18.
constexpr size_t kAnglesXyz = 0x18;
// The command's InButtonState sits at +0x70, its held, changed and scroll
// masks after the vtable.
constexpr size_t kCommandButtons = 0x78;

safetyhook::InlineHook* g_hook = nullptr;
BotDrive::Decide* g_decide = nullptr;
// g_held is each slot's buttons in the last command driven, so a button that
// goes up or down reads as changed.
std::array<uint64_t, kMaxPlayerSlots> g_held{};
bool g_reported_bare = false;

template <typename T>
T Load(const std::byte* base, size_t offset) {
  T value;
  std::memcpy(&value, base + offset, sizeof(value));
  return value;
}

template <typename T>
void Store(std::byte* base, size_t offset, const T& value) {
  std::memcpy(base + offset, &value, sizeof(value));
}

// Write puts the drive's controls in one command. A bot's brain builds its
// base message on its first tick; a command without one keeps its own axes and
// view this tick.
void Write(std::byte* command, const BotDrive::Command& drive, uint64_t& last_held) {
  // Write the held and pressed buttons.
  const uint64_t held = drive.held | drive.pressed;
  const std::array<uint64_t, 3> buttons{held, (held ^ last_held) | drive.pressed, 0};
  Store(command, kCommandButtons, buttons);
  last_held = held;

  // Write the axes and view into the base message.
  auto* base = Load<std::byte*>(command, kCommandBase);
  if (!base) {
    if (!std::exchange(g_reported_bare, true)) {
      std::fprintf(stderr, "[modlock] bot drive: a command arrived without movement fields\n");
    }
    return;
  }
  Store(command, kCommandHasBits, Load<uint32_t>(command, kCommandHasBits) | 1u);
  Store(base, kBaseHasBits, Load<uint32_t>(base, kBaseHasBits) | kBaseAxesBits);
  Store(base, kBaseForward, drive.forward);
  Store(base, kBaseLeft, drive.left);
  if (auto* angles = Load<std::byte*>(base, kBaseAngles)) {
    Store(angles, kAnglesXyz, drive.angles);
  }
}

int ProcessThunk(void* controller, std::byte* commands, int count, bool paused, float margin) {
  const auto handle = controller ? ReferenceHandleOf(controller) : std::nullopt;
  const int32_t slot = handle ? static_cast<int32_t>(EntityIndexOf(*handle)) - 1 : -1;
  if (IsPlayerSlot(slot) && commands && count > 0) {
    if (auto drive = (*g_decide)(slot)) {
      for (int i = 0; i < count; ++i) {
        Write(commands + static_cast<size_t>(i) * kCommandStride, *drive, g_held[slot]);
        // A press counts once, in the batch's first command.
        drive->pressed = 0;
      }
    } else {
      g_held[slot] = 0;
    }
  }
  return g_hook->call<int>(controller, commands, count, paused, margin);
}
#endif

}  // namespace

struct BotDrive::Impl {
#if defined(_WIN32)
  safetyhook::InlineHook hook;
  Decide decide;

  ~Impl() {
    // Only the installed hook clears the shared state.
    if (g_hook != &hook) {
      return;
    }
    hook.reset();
    g_hook = nullptr;
    g_decide = nullptr;
    g_held = {};
  }
#endif
};

BotDrive::BotDrive(std::unique_ptr<Impl> impl) : impl_(std::move(impl)) {}

BotDrive::BotDrive(BotDrive&&) noexcept = default;
BotDrive& BotDrive::operator=(BotDrive&&) noexcept = default;
BotDrive::~BotDrive() = default;

std::expected<BotDrive, std::string> BotDrive::Install(const ModuleImage& server, Decide decide) {
#if defined(_WIN32)
  // Install once.
  if (g_hook || !decide) {
    return std::unexpected("bot drive is already installed or unbound");
  }

  // Resolve the target and create the hook disabled.
  auto target = ResolveSignature(server, "controller.process-usercmds");
  if (!target) {
    return std::unexpected(target.error());
  }
  auto hook = safetyhook::create_inline(*target, reinterpret_cast<void*>(&ProcessThunk),
                                        safetyhook::InlineHook::StartDisabled);
  if (!hook) {
    return std::unexpected("bot drive hook creation failed");
  }

  // Publish the thunk state and enable.
  auto impl = std::make_unique<Impl>();
  impl->hook = std::move(hook);
  impl->decide = std::move(decide);
  g_hook = &impl->hook;
  g_decide = &impl->decide;
  if (auto enabled = impl->hook.enable(); !enabled) {
    return std::unexpected("bot drive hook activation failed");
  }
  return BotDrive(std::move(impl));

#else
  // Other platforms have no native server.
  (void)server;
  (void)decide;
  return std::unexpected("native bot control requires the Windows host");
#endif
}

}  // namespace modlock::gameinterop

#include "modlock/gameinterop/user_message_filter.h"

#include <optional>
#include <utility>

#include "modlock/gameinterop/mapped_module_image.h"
#include "modlock/gameinterop/thunk_owner.h"
#include "modlock/gameinterop/virtual_slots.h"
#include "native_network_messages.h"

#if defined(_WIN32)
#include <windows.h>
#endif

namespace modlock::gameinterop {
namespace {

#if defined(_WIN32)
using Post = void (*)(void*, int32_t, bool, int32_t, const uint64_t*, void*, const void*,
                      unsigned long, int32_t);
// g_original stays set after removal, so a hook stacked later that still
// forwards here keeps reaching the engine.
Post g_original = nullptr;
UserMessageFilter::Withheld* g_withheld = nullptr;
DWORD g_thread = 0;

void ClearThunkState() {
  g_withheld = nullptr;
  g_thread = 0;
}

// PostThunk sends the message to every recipient but the withheld ones. The
// client count is the mask's bit extent, so a post to 64 or fewer slots keeps
// its recipients in the first mask word.
void PostThunk(void* self, int32_t split_screen, bool local_only, int32_t clients,
               const uint64_t* recipients, void* serializer, const void* message,
               unsigned long size, int32_t buffer) {
  if (g_withheld && GetCurrentThreadId() == g_thread && serializer && recipients &&
      clients > 0 && clients <= 64) {
    const uint64_t withheld = (*g_withheld)(NetworkMessageIdOf(serializer)) & *recipients;
    if (withheld) {
      const uint64_t kept = *recipients & ~withheld;
      if (kept)
        g_original(self, split_screen, local_only, clients, &kept, serializer, message, size,
                   buffer);
      return;
    }
  }
  g_original(self, split_screen, local_only, clients, recipients, serializer, message, size,
             buffer);
}
#endif

}  // namespace

struct UserMessageFilter::Impl {
  Withheld withheld;
  // Restore dispatch and release the borrowed callback before destroying it.
  std::optional<ThunkOwner> owner;
};

UserMessageFilter::UserMessageFilter(std::unique_ptr<Impl> impl) : impl_(std::move(impl)) {}
UserMessageFilter::UserMessageFilter(UserMessageFilter&&) noexcept = default;
UserMessageFilter& UserMessageFilter::operator=(UserMessageFilter&&) noexcept = default;
UserMessageFilter::~UserMessageFilter() = default;

std::expected<UserMessageFilter, std::string> UserMessageFilter::Install(Withheld withheld) {
#if defined(_WIN32)
  if (g_withheld || !withheld)
    return std::unexpected("user message filter already installed or empty");
  const auto events = ResolveEngineInterface(L"engine2.dll", "GameEventSystemServerV001");
  if (!events) return std::unexpected(events.error());
  const auto post_slot = ResolveVirtualSlot("game-event-system.post-event");
  if (!post_slot) return std::unexpected(post_slot.error());
  auto impl = std::make_unique<Impl>();
  impl->withheld = std::move(withheld);
  auto hook = VtableSlotHook::Install(*events, *post_slot, reinterpret_cast<void*>(&PostThunk));
  if (!hook) return std::unexpected(hook.error());
  g_original = reinterpret_cast<Post>(hook->Original());
  impl->owner.emplace(std::move(*hook), &ClearThunkState);
  g_thread = GetCurrentThreadId();
  g_withheld = &impl->withheld;
  return UserMessageFilter(std::move(impl));
#else
  (void)withheld;
  return std::unexpected("user message filtering requires the Windows host");
#endif
}

}  // namespace modlock::gameinterop

#include "modlock/gameinterop/modifier_states.h"

#include "modlock/gameinterop/mapped_module_image.h"

namespace modlock::gameinterop {
namespace {

// kMaxModifierState bounds a plausible enumerator value; the native state
// mask holds a few hundred bits.
constexpr int64_t kMaxModifierState = 4096;

}  // namespace

std::expected<uint32_t, std::string> ModifierStateIndex(const ModuleImage& image,
                                                        std::string_view name) {
  auto value = EnumeratorValue(image, name, kMaxModifierState);
  if (!value) {
    return std::unexpected("modifier state " + value.error());
  }
  return *value;
}

std::expected<std::vector<uint32_t>, std::string> ModifierStateIndices(
    const ModuleImage& image, std::span<const std::string_view> names) {
  std::vector<uint32_t> out;
  out.reserve(names.size());
  for (const auto name : names) {
    auto index = ModifierStateIndex(image, name);
    if (!index) {
      return std::unexpected(index.error());
    }
    out.push_back(*index);
  }
  return out;
}

std::expected<std::vector<uint32_t>, std::string> ServerModifierStates(
    std::span<const std::string_view> names) {
  auto image = MappedModuleImage::ForModule(L"server.dll");
  if (!image) {
    return std::unexpected(image.error());
  }
  return ModifierStateIndices(*image, names);
}

}  // namespace modlock::gameinterop

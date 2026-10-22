#pragma once

#include <cstddef>
#include <cstdint>
#include <cstring>
#include <expected>
#include <filesystem>
#include <span>
#include <string>
#include <string_view>
#include <unordered_set>
#include <vector>

#include "gameinterop/schema_layout.h"
#include "modlock/gameinterop/console_variables.h"
#include "modlock/gameinterop/pe_layout.h"
#include "proto/modlock/dump/dump.pb.h"

namespace modlock::host_app {

// The game dump's shared pieces: one loaded module image and the listings

// built from it. game_dump.cc writes them; each listing has its own file.

namespace fs = std::filesystem;

namespace pe = gameinterop::pe;

namespace schema = gameinterop::schema;

// kRecordStride is the step of a scan for static records, which start on
// pointer boundaries.
inline constexpr size_t kRecordStride = sizeof(void*);

// Load copies a T from bytes, which need not be aligned.
template <typename T>
inline T Load(const uint8_t* bytes) {
  T value;
  std::memcpy(&value, bytes, sizeof(value));
  return value;
}

// Text copies a game string that may be null.
inline std::string Text(const char* text) {
  return text ? text : "";
}

// Image is one loaded game module.
struct Image {
  // file is the module's file name, such as server.dll.
  std::string file;
  std::span<const uint8_t> bytes;

  // kMaxNameLength bounds the scan for a name's terminator.
  static constexpr size_t kMaxNameLength = 512;

  // Contains reports whether pointer lies inside the image.
  bool Contains(const void* pointer) const {
    const auto address = reinterpret_cast<uintptr_t>(pointer);
    const auto base = reinterpret_cast<uintptr_t>(bytes.data());
    return address >= base && address < base + bytes.size();
  }

  // Name returns the printable string at text when it lies in this image,
  // or nothing. Schema and entity records keep their names beside them.
  std::string_view Name(const char* text) const {
    if (!Contains(text)) {
      return {};
    }
    const auto* end = reinterpret_cast<const char*>(bytes.data() + bytes.size());
    for (const char* at = text; at < end && size_t(at - text) < kMaxNameLength; ++at) {
      if (*at == '\0') {
        return {text, size_t(at - text)};
      }
      if (*at < 0x20 || *at > 0x7e) {
        return {};
      }
    }
    return {};
  }

  // FileHeader returns the PE file header, which the PE32+ optional header
  // follows, or null when the image has none.
  const uint8_t* FileHeader() const {
    if (bytes.size() < pe::kDosHeaderSize) {
      return nullptr;
    }
    const auto offset =
        size_t(Load<uint32_t>(bytes.data() + pe::kNtHeadersOffset)) + pe::kSignatureSize;
    const bool fits = offset + pe::kFileHeaderSize + pe::kOptionalHeaderSize <= bytes.size();
    return fits ? bytes.data() + offset : nullptr;
  }

  // Export returns the address of the named export, or zero. The loader has
  // validated the export directory of a mapped image.
  uintptr_t Export(std::string_view name) const {
    // Find the export directory through the optional header's data directory.
    const auto* file_header = FileHeader();
    if (!file_header) {
      return 0;
    }
    const auto directory =
        Load<uint32_t>(file_header + pe::kFileHeaderSize + pe::kExportDirectoryOffset);
    if (!directory || size_t(directory) + pe::kExportDirectorySize > bytes.size()) {
      return 0;
    }

    // Match the name, then map its ordinal to the function's address.
    const auto* exports = bytes.data() + directory;
    const auto* functions = bytes.data() + Load<uint32_t>(exports + pe::kExportFunctionsOffset);
    const auto* names = bytes.data() + Load<uint32_t>(exports + pe::kExportNamesOffset);
    const auto* ordinals = bytes.data() + Load<uint32_t>(exports + pe::kExportOrdinalsOffset);
    for (uint32_t i = 0, count = Load<uint32_t>(exports + pe::kExportNameCountOffset); i < count;
         ++i) {
      const auto* text =
          reinterpret_cast<const char*>(bytes.data() + Load<uint32_t>(names + 4 * i));
      if (Name(text) != name) {
        continue;
      }
      const auto ordinal = Load<uint16_t>(ordinals + 2 * i);
      return reinterpret_cast<uintptr_t>(bytes.data() + Load<uint32_t>(functions + 4 * ordinal));
    }
    return 0;
  }

  // WritableSections returns the PE sections a module's static records live
  // in once their constructors have run.
  std::vector<std::span<const uint8_t>> WritableSections() const {
    // Read the section table from the file header.
    std::vector<std::span<const uint8_t>> sections;
    const auto* file_header = FileHeader();
    if (!file_header) {
      return sections;
    }
    const auto count = Load<uint16_t>(file_header + pe::kSectionCountOffset);
    const auto* section = file_header + pe::kFileHeaderSize +
                          Load<uint16_t>(file_header + pe::kOptionalHeaderSizeOffset);

    // Keep each writable section that lies inside the image.
    for (uint16_t i = 0; i < count; ++i, section += pe::kSectionHeaderSize) {
      // Collect each writable section that fits.
      if (section + pe::kSectionHeaderSize > bytes.data() + bytes.size()) {
        break;
      }
      const auto size = Load<uint32_t>(section + pe::kSectionVirtualSizeOffset);
      const auto address = Load<uint32_t>(section + pe::kSectionVirtualAddressOffset);
      const bool writable =
          Load<uint32_t>(section + pe::kSectionCharacteristicsOffset) & pe::kSectionWrite;
      if (writable && size_t(address) + size <= bytes.size()) {
        sections.push_back(bytes.subspan(address, size));
      }
    }
    return sections;
  }
};

// SchemaRecords are the class and enum records a module declares.
struct SchemaRecords {
  std::vector<const schema::ClassInfo*> classes;
  std::vector<const schema::EnumInfo*> enums;
};

// SchemaRecords returns the class and enum records a module declares. Both
// begin with a pointer to themselves, their name and their project; the
// module's type scope confirms each candidate by finding it by name.
SchemaRecords FindSchemaRecords(void* schema_system, const Image& image);

// DescribeSchemas lists the classes and enums ordered by module and name.
dump::Schemas DescribeSchemas(SchemaRecords records);

// DescribeEntities lists the server's entity classes, the data maps they
// reach, and the entity inputs and outputs among its Pulse bindings.
// server_classes holds the server's schema classes.
std::expected<dump::Entities, std::string> DescribeEntities(
    const Image& server, const Image& tier0, const std::unordered_set<const void*>& server_classes);

// DescribeConsole lists the console variables and commands ordered by name.
std::expected<dump::Console, std::string> DescribeConsole(
    const gameinterop::ConsoleVariables& console);

}  // namespace modlock::host_app

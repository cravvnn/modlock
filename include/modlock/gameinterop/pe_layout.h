#pragma once

#include <cstddef>
#include <cstdint>

// The byte offsets of the PE32+ header fields the framework reads from game
// modules, as winnt.h declares them, for readers that parse an image's bytes
// rather than the loader's structures.
namespace modlock::gameinterop::pe {

// The DOS header ends at 0x40; e_lfanew at 0x3c locates the "PE\0\0"
// signature, which the file header follows.
inline constexpr size_t kDosHeaderSize = 0x40;
inline constexpr size_t kNtHeadersOffset = 0x3c;
inline constexpr uint32_t kSignature = 0x00004550;
inline constexpr size_t kSignatureSize = 4;

// IMAGE_FILE_HEADER fields, from the file header.
inline constexpr size_t kSectionCountOffset = 2;
inline constexpr size_t kOptionalHeaderSizeOffset = 16;
inline constexpr size_t kFileHeaderSize = 20;

// IMAGE_OPTIONAL_HEADER64 fields, from the optional header.
inline constexpr size_t kImageBaseOffset = 24;
inline constexpr size_t kImageSizeOffset = 56;
inline constexpr size_t kHeadersSizeOffset = 60;
inline constexpr size_t kExportDirectoryOffset = 112;
inline constexpr size_t kOptionalHeaderSize = 240;

// IMAGE_EXPORT_DIRECTORY fields.
inline constexpr size_t kExportNameCountOffset = 24;
inline constexpr size_t kExportFunctionsOffset = 28;
inline constexpr size_t kExportNamesOffset = 32;
inline constexpr size_t kExportOrdinalsOffset = 36;
inline constexpr size_t kExportDirectorySize = 40;

// IMAGE_SECTION_HEADER fields and characteristics.
inline constexpr size_t kSectionHeaderSize = 40;
inline constexpr size_t kSectionVirtualSizeOffset = 8;
inline constexpr size_t kSectionVirtualAddressOffset = 12;
inline constexpr size_t kSectionRawSizeOffset = 16;
inline constexpr size_t kSectionRawOffsetOffset = 20;
inline constexpr size_t kSectionCharacteristicsOffset = 36;
inline constexpr uint32_t kSectionExecute = 0x20000000;
inline constexpr uint32_t kSectionWrite = 0x80000000;

}  // namespace modlock::gameinterop::pe

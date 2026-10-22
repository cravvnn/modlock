#include <algorithm>
#include <cstdint>
#include <string_view>
#include <utility>

#include "host_app/game_dump_internal.h"
#include "modlock/gameinterop/entity_abi.h"

namespace modlock::host_app {

namespace {

// The class flags a dump names, from SCHEMA_CF1_*. Other bits describe the
// schema system's own bookkeeping.
constexpr std::pair<uint32_t, std::string_view> kClassFlags[] = {
    {1u << 1, "abstract"},
    {1u << 2, "trivial_constructor"},
    {1u << 3, "trivial_destructor"},
    {1u << 9, "construct_disallowed"},
    {1u << 10, "MNetworkAssumeNotNetworkable"},
    {1u << 11, "MNetworkNoBase"},
    {1u << 12, "MIgnoreTypeScopeMetaChecks"},
    {1u << 13, "MDisableDataDescValidation"},
    {1u << 14, "MClassHasEntityLimitedDataDesc"},
    {1u << 15, "MClassHasCustomAlignedNewDelete"},
    {1u << 16, "MNonConstructibleClassBase"},
    {1u << 17, "MConstructibleClassBase"},
    {1u << 18, "MHasKV3TransferPolymorphicClassname"},
};

// AddMetadata adds the names of a schema record's metadata entries.
template <typename Repeated>
void AddMetadata(const schema::MetadataEntry* entries, int32_t count, Repeated* names) {
  for (int32_t i = 0; i < count; ++i) {
    names->Add(Text(entries[i].name));
  }
}

// ByModuleAndName orders schema records by declaring module, then name.
template <typename Record>
bool ByModuleAndName(const Record* a, const Record* b) {
  return std::tuple(std::string_view(a->project), std::string_view(a->name)) <
         std::tuple(std::string_view(b->project), std::string_view(b->name));
}

// AddClass describes a schema class: its layout, bases, fields and flags.
void AddClass(const schema::ClassInfo& info, dump::SchemaClass& out) {
  // Copy the class's identity and layout.
  out.set_name(info.name);
  out.set_module(info.project);
  out.set_size(static_cast<uint32_t>(info.size));
  out.set_alignment(info.alignment);

  // List its bases and fields in declaration order.
  for (const auto& base : std::span(info.bases, info.base_count)) {
    auto& added = *out.add_bases();
    added.set_name(Text(base.info->name));
    added.set_module(Text(base.info->project));
    added.set_offset(base.offset);
  }
  for (const auto& field : std::span(info.fields, info.field_count)) {
    // Add each field with its type, offset and metadata.
    auto& added = *out.add_fields();
    added.set_name(Text(field.name));
    added.set_type(field.type ? Text(field.type->name) : "");
    added.set_offset(static_cast<uint32_t>(field.offset));
    AddMetadata(field.metadata, field.metadata_count, added.mutable_metadata());
  }

  // Name its metadata and the flags a dump reports.
  AddMetadata(info.metadata, info.metadata_count, out.mutable_metadata());
  for (const auto& [bit, name] : kClassFlags) {
    if (info.flags & bit) {
      out.add_flags(std::string(name));
    }
  }
}

// AddEnum describes a schema enum and its enumerators.
void AddEnum(const schema::EnumInfo& info, dump::SchemaEnum& out) {
  // Copy the enum's name, size, enumerators and metadata.
  out.set_name(info.name);
  out.set_module(info.project);
  out.set_size(info.size);
  for (const auto& enumerator : std::span(info.enumerators, info.enumerator_count)) {
    auto& added = *out.add_enumerators();
    added.set_name(Text(enumerator.name));
    added.set_value(enumerator.value);
  }
  AddMetadata(info.metadata, info.metadata_count, out.mutable_metadata());
}

}  // namespace

SchemaRecords FindSchemaRecords(void* schema_system, const Image& image) {
  // Find the module's type scope.
  SchemaRecords records;
  auto scope = gameinterop::FindTypeScopeOf(schema_system, image.file.c_str());
  if (!scope) {
    return records;
  }

  // A record that points to itself and is declared in the scope is a class or an enum.
  for (const auto section : image.WritableSections()) {
    for (size_t offset = 0; offset + sizeof(schema::EnumInfo) <= section.size();
         offset += kRecordStride) {
      // A self-pointing record names a class or enum the scope declares.
      const auto* record = section.data() + offset;
      if (Load<const void*>(record) != record) {
        continue;
      }
      const auto* info = reinterpret_cast<const schema::ClassInfo*>(record);
      if (image.Name(info->name).empty() || image.Name(info->project).empty()) {
        continue;
      }
      if (auto found = gameinterop::FindDeclaredClassOf(*scope, info->name);
          found && *found == info) {
        records.classes.push_back(info);
      } else if (auto found = gameinterop::FindDeclaredEnumOf(*scope, info->name);
                 found && *found == record) {
        records.enums.push_back(reinterpret_cast<const schema::EnumInfo*>(record));
      }
    }
  }
  return records;
}

dump::Schemas DescribeSchemas(SchemaRecords records) {
  // Order the records by module and name, then describe each.
  std::ranges::sort(records.classes, ByModuleAndName<schema::ClassInfo>);
  std::ranges::sort(records.enums, ByModuleAndName<schema::EnumInfo>);
  dump::Schemas schemas;
  for (const auto* info : records.classes) {
    AddClass(*info, *schemas.add_classes());
  }
  for (const auto* info : records.enums) {
    AddEnum(*info, *schemas.add_enums());
  }
  return schemas;
}

}  // namespace modlock::host_app

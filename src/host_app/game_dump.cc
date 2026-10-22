#include "host_app/game_dump.h"

#include <google/protobuf/json/json.h>

#include <algorithm>
#include <fstream>
#include <string_view>
#include <unordered_set>
#include <utility>
#include <vector>

#include "host_app/game_dump_internal.h"
#include "modlock/gameinterop/entity_abi.h"
#include "modlock/gameinterop/mapped_module_image.h"

namespace modlock::host_app {

namespace {

// LoadedModules returns the game modules the server has mapped.
std::vector<Image> LoadedModules(const GamePaths& paths) {
  std::vector<Image> images;
  for (const auto& directory : {paths.engine2_dll.parent_path(), paths.server_dll.parent_path()}) {
    std::error_code error;
    for (const auto& entry : fs::directory_iterator(directory, error)) {
      // Map each new DLL in the directory.
      if (entry.path().extension() != ".dll") {
        continue;
      }
      const auto file = entry.path().filename();
      auto image = gameinterop::MappedModuleImage::ForModule(file.wstring());
      if (!image) {
        continue;
      }
      if (std::ranges::any_of(images, [&](const Image& seen) { return seen.file == file; })) {
        continue;
      }
      images.push_back({file.string(), image->image_bytes()});
    }
  }
  return images;
}

// Build reads the install's ClientVersion from steam.inf.
std::string Build(const GamePaths& paths) {
  // Find the ClientVersion line in the install's steam.inf.
  constexpr std::string_view kClientVersion = "ClientVersion=";
  std::ifstream in(paths.server_dll.parent_path().parent_path().parent_path() / "steam.inf");
  std::string line;
  while (std::getline(in, line)) {
    if (line.starts_with(kClientVersion)) {
      return line.substr(kClientVersion.size());
    }
  }
  return "unknown";
}

// WriteJson writes message to path as indented JSON with proto field names.
std::expected<void, std::string> WriteJson(const google::protobuf::Message& message,
                                           const fs::path& path) {
  // Print the message as indented JSON.
  std::string text;
  google::protobuf::json::PrintOptions options;
  options.add_whitespace = true;
  options.preserve_proto_field_names = true;
  if (auto printed = google::protobuf::json::MessageToJsonString(message, &text, options);
      !printed.ok()) {
    return std::unexpected(path.filename().string() + ": " + std::string(printed.message()));
  }

  // Write it to the file.
  std::ofstream out(path, std::ios::binary);
  out << text;
  if (!out) {
    return std::unexpected("cannot write " + path.string());
  }
  return {};
}

}  // namespace

std::expected<GameDumpCounts, std::string> WriteGameDump(const GamePaths& paths,
                                                         const fs::path& directory) {
  // Resolve the schema system and the console, and make the directory.
  auto schema_system = gameinterop::ResolveSchemaSystem();
  if (!schema_system) {
    return std::unexpected(schema_system.error());
  }
  auto console = gameinterop::ConsoleVariables::Resolve();
  if (!console) {
    return std::unexpected(console.error());
  }
  std::error_code error;
  fs::create_directories(directory, error);
  if (error) {
    return std::unexpected("cannot create " + directory.string() + ": " + error.message());
  }

  // Gather the schema records of every loaded module.
  SchemaRecords records;
  std::unordered_set<const void*> server_classes;
  const Image* server = nullptr;
  const Image* tier0 = nullptr;
  const auto images = LoadedModules(paths);
  for (const auto& image : images) {
    // Collect each module's records, noting the server and tier0.
    auto found = FindSchemaRecords(*schema_system, image);
    if (image.file == paths.server_dll.filename()) {
      server = &image;
      server_classes.insert(found.classes.begin(), found.classes.end());
    }
    if (image.file == "tier0.dll") {
      tier0 = &image;
    }
    records.classes.insert(records.classes.end(), found.classes.begin(), found.classes.end());
    records.enums.insert(records.enums.end(), found.enums.begin(), found.enums.end());
  }
  if (!server || !tier0) {
    return std::unexpected("server.dll or tier0.dll is not loaded");
  }

  // Describe the schemas, entities and console.
  auto schemas = DescribeSchemas(std::move(records));
  auto entities = DescribeEntities(*server, *tier0, server_classes);
  if (!entities) {
    return std::unexpected(entities.error());
  }
  auto console_listing = DescribeConsole(*console);
  if (!console_listing) {
    return std::unexpected(console_listing.error());
  }

  // Stamp each listing with the game build.
  const auto build = Build(paths);
  schemas.set_build(build);
  entities->set_build(build);
  console_listing->set_build(build);

  // Write each listing to its file.
  const std::pair<const google::protobuf::Message*, std::string_view> listings[] = {
      {&schemas, "schemas.json"},
      {&*entities, "entities.json"},
      {&*console_listing, "console.json"},
  };
  for (const auto& [message, file] : listings) {
    if (auto written = WriteJson(*message, directory / file); !written) {
      return std::unexpected(written.error());
    }
  }
  return GameDumpCounts{
      .classes = size_t(schemas.classes_size()),
      .enums = size_t(schemas.enums_size()),
      .designer_names = size_t(entities->classes_size()),
      .data_maps = size_t(entities->data_maps_size()),
      .inputs = size_t(entities->inputs_size()),
      .outputs = size_t(entities->outputs_size()),
      .variables = size_t(console_listing->variables_size()),
      .commands = size_t(console_listing->commands_size()),
  };
}

}  // namespace modlock::host_app

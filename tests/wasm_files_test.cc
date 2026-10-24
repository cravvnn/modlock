// Contract test for a mod's files: the written copy over the shipped one,
// the names the host refuses and the quota.
#include <filesystem>
#include <fstream>

#include "gtest/gtest.h"
#include "wasm/files.h"

namespace modlock::wasm {
namespace {

TEST(ModFiles, WrittenOverShippedWithinTheMod) {
  // Ship one file and keep the mod's writes beside the build.
  const auto root = std::filesystem::path(::testing::TempDir()) / "mod_files";
  std::filesystem::remove_all(root);
  std::filesystem::create_directories(root / "data" / "maps");
  std::ofstream(root / "data" / "maps" / "areas.yaml") << "shipped";
  const ModFiles files{.written = root / "files" / "hud", .data = root / "data"};

  // A read finds the shipped file until the mod writes its own, and a missing
  // file has no text.
  EXPECT_EQ(files.Read("maps/areas.yaml").value(), "shipped");
  ASSERT_TRUE(files.Write("maps/areas.yaml", "written").has_value());
  EXPECT_EQ(files.Read("maps/areas.yaml").value(), "written");
  EXPECT_EQ(files.Read("missing.yaml").value(), std::nullopt);

  // Names that leave the mod's files are refused, through a link as well.
  for (const auto* name : {"", "/etc/passwd", "../other/x", "a//b", "./a", "a\\b", "c:x"}) {
    EXPECT_FALSE(files.Write(name, "x").has_value()) << name;
    EXPECT_FALSE(files.Read(name).has_value()) << name;
  }
  std::filesystem::create_directory_symlink(root, root / "files" / "hud" / "out");
  EXPECT_FALSE(files.Write("out/escaped", "x").has_value());
  EXPECT_FALSE(std::filesystem::exists(root / "escaped"));

  // A write past the quota is refused and leaves the previous text, and a host
  // without a directory refuses every write.
  EXPECT_FALSE(files.Write("maps/areas.yaml", std::string(kFilesQuota + 1, 'x')).has_value());
  EXPECT_EQ(files.Read("maps/areas.yaml").value(), "written");
  EXPECT_FALSE(ModFiles{.written = {}, .data = root / "data"}.Write("a", "x").has_value());
}

}  // namespace
}  // namespace modlock::wasm

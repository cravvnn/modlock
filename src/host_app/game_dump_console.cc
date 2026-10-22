#include <algorithm>
#include <cstdint>
#include <string_view>

#include "host_app/game_dump_internal.h"

namespace modlock::host_app {

namespace {

// The console flag names (FCVAR_*), indexed by bit.
constexpr std::string_view kConsoleFlags[] = {
    "linked_concommand",
    "developmentonly",
    "gamedll",
    "clientdll",
    "hidden",
    "protected",
    "sponly",
    "archive",
    "notify",
    "userinfo",
    "reference",
    "unlogged",
    "initial_setvalue",
    "replicated",
    "cheat",
    "per_user",
    "demo",
    "dontrecord",
    "performing_callbacks",
    "release",
    "menubar_item",
    "commandline_enforced",
    "notconnected",
    "vconsole_fuzzy_matching",
    "server_can_execute",
    "client_can_execute",
    "server_cannot_query",
    "vconsole_set_focus",
    "clientcmd_can_execute",
    "execute_per_tick",
    "snapshot_ignored",
    "",
    "defensive",
    "execute_immediately",
    "gameinfo_cannot_override",
    "",
    "",
    "enum_value",
};

// AddConsoleFlags names each set console flag bit, as flag_N when the game
// gives it no name.
template <typename Repeated>
void AddConsoleFlags(uint64_t flags, Repeated* names) {
  for (size_t bit = 0; bit < 64; ++bit) {
    if (!(flags & (1ull << bit))) {
      continue;
    }
    if (bit < std::size(kConsoleFlags) && !kConsoleFlags[bit].empty()) {
      names->Add(std::string(kConsoleFlags[bit]));
    } else {
      names->Add("flag_" + std::to_string(bit));
    }
  }
}

}  // namespace

std::expected<dump::Console, std::string> DescribeConsole(
    const gameinterop::ConsoleVariables& console) {
  // Read the variables and commands and sort them by name.
  auto variables = console.Variables();
  if (!variables) {
    return std::unexpected(variables.error());
  }
  auto commands = console.Commands();
  if (!commands) {
    return std::unexpected(commands.error());
  }
  std::ranges::sort(*variables, {}, &gameinterop::ConsoleVariableInfo::name);
  std::ranges::sort(*commands, {}, &gameinterop::ConsoleCommandInfo::name);

  // List each with its flags.
  dump::Console listing;
  for (const auto& variable : *variables) {
    // Add each variable with its type, bounds and help.
    auto& added = *listing.add_variables();
    added.set_name(variable.name);
    added.set_type(static_cast<dump::ConsoleVariableType>(variable.type + 1));
    added.set_default_value(variable.default_value);
    added.set_min(variable.min);
    added.set_max(variable.max);
    added.set_help(variable.help);
    AddConsoleFlags(variable.flags, added.mutable_flags());
  }
  for (const auto& command : *commands) {
    auto& added = *listing.add_commands();
    added.set_name(command.name);
    added.set_help(command.help);
    AddConsoleFlags(command.flags, added.mutable_flags());
  }
  return listing;
}

}  // namespace modlock::host_app

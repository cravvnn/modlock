#pragma once

#include <cstdint>
#include <optional>
#include <set>
#include <string>
#include <string_view>

#include "src/wasm/players/players.pb.h"
#include "src/wasm/wasm.pb.h"

namespace modlock::wasm {

// PlayerScope limits a mod to a set of players, as when one mod runs a game
// for some of a server's players beside another mod. A limited mod sees only
// its players in the player list, hears no event about another player, and
// is refused a request that addresses another player or changes the whole
// server. An unlimited scope serves every player.
//
// A message field named player or players holds client slots; -1 names no
// player. The scope reads those fields from each request and event by its
// method's message type, so new methods are covered without a list. Entities
// and world objects are not players: a limited mod still reaches them.
class PlayerScope {
 public:
  // Limit serves only players, or every player when players is nullopt.
  void Limit(std::optional<std::set<int32_t>> players) { players_ = std::move(players); }

  // Limited reports whether the scope serves only some players.
  [[nodiscard]] bool Limited() const { return players_.has_value(); }

  // Serves reports whether the scope serves the player in slot.
  [[nodiscard]] bool Serves(int32_t slot) const {
    return !players_ || slot < 0 || players_->contains(slot);
  }

  // Refuse returns why the mod may not make the host request call, or an
  // empty string when it may.
  [[nodiscard]] std::string Refuse(const Call& call) const;

  // Hears reports whether the mod hears the event call.
  [[nodiscard]] bool Hears(const Call& event) const;

  // Keep drops the players the scope does not serve from a player list.
  void Keep(google::protobuf::RepeatedPtrField<players::Connection>& players) const;

 private:
  // Addresses reports whether message, of the named method's request type in
  // service, names only players the scope serves.
  [[nodiscard]] bool Addresses(std::string_view service, std::string_view method,
                               const std::string& message) const;

  std::optional<std::set<int32_t>> players_;
};

}  // namespace modlock::wasm

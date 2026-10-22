#pragma once

#include <cstddef>
#include <cstdint>
#include <expected>
#include <span>
#include <string>

#include "modlock/gameinterop/pawn_observer.h"

namespace modlock::gameinterop {

// The pieces of PawnObserver its translation units share.

// ScoreCounter names one PlayerDataGlobal_t scoreboard counter, its offset in
// ScoreLayout and its value in ScoreTotals.
struct ScoreCounter {
  const char* field_name;
  size_t ScoreLayout::* offset;
  int32_t ScoreTotals::* total;
};

// kScoreCounters lists every scoreboard counter the observer reads.
inline constexpr ScoreCounter kScoreCounters[] = {
    {"m_iHeroDamage", &ScoreLayout::hero_damage, &ScoreTotals::hero_damage},
    {"m_iHeroHealing", &ScoreLayout::hero_healing, &ScoreTotals::hero_healing},
    {"m_iSelfHealing", &ScoreLayout::self_healing, &ScoreTotals::self_healing},
    {"m_iPlayerKills", &ScoreLayout::kills, &ScoreTotals::kills},
    {"m_iDeaths", &ScoreLayout::deaths, &ScoreTotals::deaths},
    {"m_iPlayerAssists", &ScoreLayout::assists, &ScoreTotals::assists},
    {"m_iLastHits", &ScoreLayout::last_hits, &ScoreTotals::last_hits},
    {"m_iDenies", &ScoreLayout::denies, &ScoreTotals::denies},
    {"m_iGoldNetWorth", &ScoreLayout::net_worth, &ScoreTotals::net_worth},
};

// CNetworkViewOffsetVector stores its three network floats eight bytes apart,
// starting 0x10 into the field.
inline constexpr size_t kViewOffsetFirstFloat = 0x10;
inline constexpr size_t kViewOffsetFloatStride = 8;
inline constexpr size_t kViewOffsetSize =
    kViewOffsetFirstFloat + 2 * kViewOffsetFloatStride + sizeof(float);

// EntityListOf reads the concrete entity list out of one live CGameEntitySystem.
unsigned char* EntityListOf(void* entity_system);

// IdentityOfIndex reproduces CEntitySystem::GetEntityIdentity(CEntityIndex)
// (sourcesdk entity2/entitysystem.cpp): chunk-table pick plus the identity's
// own index proof, so a stale identity slot never resolves.
unsigned char* IdentityOfIndex(unsigned char* entity_list, uint32_t index);

// IdentityOfHandle reproduces CEntitySystem::GetEntityIdentity(CEntityHandle):
// same chunk-table pick, then GetRefEHandle proof where the stored serial is
// corrected by EF_IS_INVALID_EHANDLE before comparison.
unsigned char* IdentityOfHandle(unsigned char* entity_list, uint32_t handle);

// InstanceOf reads the entity instance an identity points to.
void* InstanceOf(unsigned char* identity);

// AbsOriginOf follows a pawn's body component to its scene node's
// absolute origin, or is null when the chain is incomplete.
const float* AbsOriginOf(const EntityLayout& layout, void* pawn);

// AllFinite reports whether every value is finite.
bool AllFinite(std::span<const float> values);

// ResolveEntityLayout reads core offsets through the game's own schema system.
// Each optional field is optional: a missing field or a storage span too small
// for its direct type omits only that field and leaves core observation usable.
std::expected<EntityLayout, std::string> ResolveEntityLayout(bool movement_enabled);

// ResolveAbilityLayout resolves the ability fields the observer reads and
// writes through the game's schema system.
std::expected<AbilityLayout, std::string> ResolveAbilityLayout();

// PrepareUpgradeBonuses locates the upgrade bonus modifier's layout ahead of
// its first use, since locating it costs a frame.
void PrepareUpgradeBonuses();

}  // namespace modlock::gameinterop

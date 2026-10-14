#pragma once

#include <cstddef>
#include <expected>
#include <string>
#include <string_view>
#include <vector>

#include "modlock/export.h"
#include "modlock/gameinterop/game_symbols.h"

namespace modlock::gameinterop {

// VirtualSlot is one virtual function modlock calls or hooks by its index in a
// class's dispatch table. A game update that adds or removes a virtual ahead of
// it moves the function and leaves a different one at the index, so a call
// through a stale index reaches the wrong function without any error.
// modlock-sigcheck checks every entry of kVirtualSlots against the game files:
// the class's table, found through its RTTI name, must hold a function at index
// whose entry bytes match pattern, and no other function of the table may
// match it.
struct VirtualSlot {
  // module is the file that defines the class, such as "server.dll".
  std::string_view module;
  // class_name is the implementing class's RTTI name, such as "CBaseEntity".
  std::string_view class_name;
  // index is the function's position in the class's primary table.
  size_t index = 0;
  // pattern matches the function's entry bytes in the ParseSignature syntax.
  std::string_view pattern;
  // shape names the function.
  std::string_view shape;
};

// IVEngineServer2 counts IAppSystem's eleven slots, ISource2Engine's seven,
// then its own declarations (sourcesdk public/eiface.h).
inline constexpr VirtualSlot kServerCommandSlot{
    "engine2.dll", "CEngineServer", 45, "48 8B 0D ?? ?? ?? ?? 4C 8B C2",
    "void IVEngineServer2::ServerCommand(const char* command)"};
inline constexpr VirtualSlot kServerGlobalsSlot{"engine2.dll", "CEngineServer", 76,
                                                "48 8B 05 ?? ?? ?? ?? 48 85 C0 74 ?? 48 83 C0 58",
                                                "CGlobalVars* IVEngineServer2::GetServerGlobals()"};

inline constexpr VirtualSlot kGetGameServerSlot{
    "engine2.dll", "CNetworkServerService", 23, "48 8B 81 50 01 00 00 C3",
    "INetworkGameServer* INetworkServerService::GetIGameServer()"};
inline constexpr VirtualSlot kStartupServerSlot{
    "engine2.dll", "CNetworkServerService", 26, "48 8B C4 55 57 41 57 48 83 EC 60",
    "void INetworkServerService::StartupServer(const GameSessionConfiguration_t&, ...)"};

// The Deadlock SDK's public/iserver.h declares Shutdown fifth.
inline constexpr VirtualSlot kGameServerShutdownSlot{"engine2.dll", "CNetworkGameServer", 4,
                                                     "40 55 48 83 EC 20 80 3D ?? ?? ?? ?? 00",
                                                     "void INetworkGameServer::Shutdown()"};

inline constexpr VirtualSlot kPostEventSlot{
    "engine2.dll", "CGameEventSystem", 16, "48 89 5C 24 10 48 89 74 24 18",
    "void IGameEventSystem::PostEventAbstract(CSplitScreenSlot, bool, int, const uint64*, "
    "INetworkMessageInternal*, const CNetMessage*, unsigned long, NetChannelBufType_t)"};

// Since game build 6711 a message serializer is a plain record, and
// INetworkMessages serializes and parses messages (sourcesdk
// networksystem/inetworkmessages.h).
inline constexpr VirtualSlot kNetworkMessagesUnserializeSlot{
    "networksystem.dll", "CNetworkMessages", 3, "48 8B C4 53 48 81 EC 90 00 00 00",
    "bool INetworkMessages::UnserializeAbstract(bf_read&, INetworkMessageInternal*, CNetMessage*)"};
inline constexpr VirtualSlot kNetworkMessagesSerializeSlot{
    "networksystem.dll", "CNetworkMessages", 4, "48 89 5C 24 18 56 48 83 EC 40",
    "bool INetworkMessages::SerializeAbstract(bf_write&, INetworkMessageInternal*, "
    "const CNetMessage*)"};
inline constexpr VirtualSlot kNetworkMessagesFindByIdSlot{
    "networksystem.dll", "CNetworkMessages", 27, "48 89 5C 24 18 48 89 6C 24 20 56",
    "INetworkMessageInternal* INetworkMessages::FindNetworkMessageById(int)"};

// ISchemaSystem appends FindTypeScopeForModule after IAppSystem's eleven
// slots; ISchemaSystemTypeScope declares FindDeclaredClass third and
// FindDeclaredEnum fourth.
inline constexpr VirtualSlot kSchemaSystemFindTypeScopeSlot{
    "schemasystem.dll", "CSchemaSystem", 13, "48 89 5C 24 10 48 89 74 24 18",
    "CSchemaSystemTypeScope* ISchemaSystem::FindTypeScopeForModule(const char*, int*)"};
inline constexpr VirtualSlot kTypeScopeFindDeclaredClassSlot{
    "schemasystem.dll", "CSchemaSystemTypeScope", 2,
    "40 53 48 83 EC 20 48 8B 01 48 8B DA 49 8B D0 FF 90 C8 00 00 00",
    "SchemaClassInfoData_t* ISchemaSystemTypeScope::FindDeclaredClass(const char*)"};
inline constexpr VirtualSlot kTypeScopeFindDeclaredEnumSlot{
    "schemasystem.dll", "CSchemaSystemTypeScope", 3,
    "40 53 48 83 EC 20 48 8B 01 48 8B DA 49 8B D0 FF 90 D8 00 00 00",
    "SchemaEnumInfoData_t* ISchemaSystemTypeScope::FindDeclaredEnum(const char*)"};

inline constexpr VirtualSlot kGameFrameSlot{
    "server.dll", "CSource2Server", 19, "48 89 5C 24 20 88 54 24 10",
    "void ISource2Server::GameFrame(bool simulating, bool first_tick, bool last_tick)"};

// ISource2GameEntities::CheckTransmit follows the eleven IAppSystem entries
// and ApplyGameSettings.
inline constexpr VirtualSlot kCheckTransmitSlot{
    "server.dll", "CSource2GameEntities", 12, "48 8B C4 4C 89 48 20 44 89 40 18",
    "void ISource2GameEntities::CheckTransmit(CCheckTransmitInfo**, int, CBitVec&, ...)"};

inline constexpr VirtualSlot kClientPutInServerSlot{
    "server.dll", "CSource2GameClients", 13, "48 89 5C 24 10 48 89 6C 24 18 56 48 83 EC 30",
    "void ISource2GameClients::ClientPutInServer(CPlayerSlot, const char*, int, uint64)"};
inline constexpr VirtualSlot kClientFullyConnectSlot{
    "server.dll", "CSource2GameClients", 15, "40 53 48 83 EC 30 48 8B 0D ?? ?? ?? ??",
    "void ISource2GameClients::ClientFullyConnect(CPlayerSlot)"};
inline constexpr VirtualSlot kClientDisconnectSlot{
    "server.dll", "CSource2GameClients", 16, "48 89 5C 24 18 48 89 74 24 20",
    "void ISource2GameClients::ClientDisconnect(CPlayerSlot, ENetworkDisconnectionReason, ...)"};
inline constexpr VirtualSlot kClientCommandSlot{
    "server.dll", "CSource2GameClients", 17, "48 89 74 24 10 57 48 81 EC 80 00 00 00",
    "void ISource2GameClients::ClientCommand(CPlayerSlot, const CCommand&)"};

// NetworkStateChanged follows RequiredEdictIndex in the current Windows ABI;
// slot 27 writes an entity index to its hidden result.
inline constexpr VirtualSlot kNetworkStateChangedSlot{
    "server.dll", "CBaseEntity", 28, "48 8B C4 56 48 81 EC 40 01 00 00",
    "void CEntityInstance::NetworkStateChanged(const NetworkStateChangedData&)"};
inline constexpr VirtualSlot kTeleportSlot{
    "server.dll", "CBaseEntity", 163, "48 89 5C 24 08 48 89 6C 24 10 48 89 74 24 18 48 89 7C 24 20",
    "void CBaseEntity::Teleport(const Vector*, const QAngle*, const Vector*)"};
// The pawn overrides GetMaxHealth to evaluate progression and modifiers; the
// base version reads m_iMaxHealth.
inline constexpr VirtualSlot kGetMaxHealthSlot{
    "server.dll", "CBaseEntity", 181, "8B 81 D4 02 00 00 C3", "int CBaseEntity::GetMaxHealth()"};
inline constexpr VirtualSlot kChangeTeamSlot{"server.dll", "CBasePlayerController", 105,
                                             "40 55 56 41 57 48 8B EC",
                                             "void CBasePlayerController::ChangeTeam(int team)"};

// kVirtualSlots lists every slot above; add each new slot here so
// modlock-sigcheck checks it.
inline constexpr VirtualSlot kVirtualSlots[] = {
    kServerCommandSlot,
    kServerGlobalsSlot,
    kGetGameServerSlot,
    kStartupServerSlot,
    kGameServerShutdownSlot,
    kPostEventSlot,
    kNetworkMessagesUnserializeSlot,
    kNetworkMessagesSerializeSlot,
    kNetworkMessagesFindByIdSlot,
    kSchemaSystemFindTypeScopeSlot,
    kTypeScopeFindDeclaredClassSlot,
    kTypeScopeFindDeclaredEnumSlot,
    kGameFrameSlot,
    kCheckTransmitSlot,
    kClientPutInServerSlot,
    kClientFullyConnectSlot,
    kClientDisconnectSlot,
    kClientCommandSlot,
    kNetworkStateChangedSlot,
    kTeleportSlot,
    kGetMaxHealthSlot,
    kChangeTeamSlot,
};

// FindVtable returns the image offset of class_name's primary dispatch table,
// found through its MSVC RTTI: the type descriptor named ".?AV<class_name>@@",
// the complete object locator that places it at offset zero, and the table
// that follows the pointer to that locator.
[[nodiscard]] MODLOCK_API std::expected<size_t, std::string> FindVtable(
    const ModuleImage& image, std::string_view class_name);

// VtableEntries returns the image offset of each function in the table at
// image offset table, ending at the first entry outside executable code.
[[nodiscard]] MODLOCK_API std::vector<size_t> VtableEntries(const ModuleImage& image, size_t table);

// MatchingSlots returns every index of slot's class table whose function's
// entry bytes match slot's pattern. A sound slot yields exactly its index.
[[nodiscard]] MODLOCK_API std::expected<std::vector<size_t>, std::string> MatchingSlots(
    const ModuleImage& image, const VirtualSlot& slot);

}  // namespace modlock::gameinterop

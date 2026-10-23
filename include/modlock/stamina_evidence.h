#pragma once

#include <cstdint>

namespace modlock {

// StaminaEvidence is a complete server-observed expenditure counter for one
// player session. Only the native consumption observer supplies it; snapshots
// of the regenerating stamina bar cannot establish this counter.
struct StaminaEvidence {
  // total_spent is cumulative consumption in bars during this epoch.
  double total_spent = 0;
  // epoch changes when observation continuity or the observed pawn changes.
  uint64_t epoch = 0;
};

}  // namespace modlock

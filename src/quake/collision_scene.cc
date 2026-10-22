#include "quake/collision_scene.h"

#include "quake/collision_hull.h"

namespace modlock::quake {

Trace CollisionScene::Sweep(const Vector& start, const Vector& end, const Vector& mins,
                            const Vector& maxs) const {
  auto closest = world_.Sweep(start, end, mins, maxs);
  const bool point = mins == Vector{} && maxs == Vector{};
  for (const auto& body : bodies_) {
    // A point trace stops inside a solid; skip the ignored body.
    if (point && closest.all_solid) {
      break;
    }
    if (body.entity == ignored_entity_) {
      continue;
    }

    // Build the body's box as six planes, grown by the moving hull.
    std::array<CollisionHull::Plane, 6> planes;
    std::array<CollisionHull::Node, 6> nodes;
    for (int index = 0; index < 6; ++index) {
      // Each face is one side of an axis, pushed out by the hull.
      const int axis = index / 2;
      const int outside = index & 1;
      auto& plane = planes[index];
      plane.axis = axis;
      plane.normal[axis] = 1;
      plane.distance = outside ? body.mins[axis] - maxs[axis] : body.maxs[axis] - mins[axis];

      // Its node leaves outside empty and goes on inside; past the last is solid.
      nodes[index].plane = index;
      nodes[index].children[outside] = -1;
      nodes[index].children[outside ^ 1] = index == 5 ? -2 : index + 1;
    }

    // Sweep the box and keep the closest hit.
    auto trace =
        CollisionHull(planes, nodes, 0)
            .Sweep(start, end, body.origin,
                   point ? CollisionHull::TraceKind::kEntity : CollisionHull::TraceKind::kPlayer);
    if (trace.fraction < closest.fraction || (point && (trace.all_solid || trace.start_solid))) {
      trace.entity = body.entity;
      if (point && closest.start_solid) {
        trace.start_solid = true;
      }
      closest = trace;
    }
  }
  return closest;
}

}  // namespace modlock::quake

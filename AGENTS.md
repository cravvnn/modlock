# Modlock engineering rules

## Frame time

Modlock runs inside the game server's frame: 64 frames a second, about 15 ms
each, shared with the game and every mod. Each host call, entity read and
allocation a frame makes is paid by every mod that uses it, every frame, so
the framework's own cost multiplies across its users.

After code works, read it over again for frame time before landing it. Count
what one frame costs: host calls, schema lookups, allocations, copies across
the sandbox boundary, and loops over entities or players. Then improve it:
resolve a field, signature or class once and keep it, answer a repeated
question from one batched call, avoid encoding work the caller does not read,
and choose the data structure that serves the lookup, such as a map or a set
in place of a scan. Keep improving until the next change saves little, and
stop where it would make the code hard to read: a clear loop over a dozen
entries beats a clever cache.

An improvement a mod makes that any mod could use, such as a cheaper host
call, a cache or a missing primitive, belongs here. Take it in as a general
API, document its cost in its comment, and release it so the mod can drop its
own copy.

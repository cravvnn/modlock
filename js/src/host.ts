// The host bridge: calls to the game and the events it delivers, through the
// runtime the mod runs on. Each runtime's entry binds its own transport, so
// the library itself reads no runtime globals.

import { Call, Reply } from '../../proto/modlock/wasm.pb.js'

/** Transport exchanges an encoded Call for the host's encoded Reply. */
export type Transport = (call: Uint8Array) => Uint8Array

/** Dispatch answers one encoded Call the host delivers with its encoded Reply. */
export type Dispatch = (call: Uint8Array) => Uint8Array

/** unbound answers every call before a runtime binds the host as refused. */
const unbound: Transport = () =>
  Reply.toBinary({ error: 'the host is not bound yet' })

/** transport carries calls to the host; the runtime's entry binds it. */
let transport: Transport = unbound

/** dispatch delivers the host's events to the library's handlers. */
let dispatch: Dispatch | undefined

/** bind routes the mod's host calls through t from now on. */
export function bind(t: Transport): void {
  transport = t
}

/** serveEvents answers each event the host delivers with d. */
export function serveEvents(d: Dispatch): void {
  dispatch = d
}

/**
 * deliver answers one encoded Call from the host. The library registers its
 * dispatch when it loads, before any event can arrive.
 */
export function deliver(call: Uint8Array): Uint8Array {
  if (dispatch === undefined) {
    return Reply.toBinary({ error: 'the mod library is not loaded' })
  }
  return dispatch(call)
}

/**
 * call calls one Host method with its encoded request and returns the
 * encoded response, or undefined when the call failed; the host logs why.
 */
export function call(
  method: string,
  request: Uint8Array,
): Uint8Array | undefined {
  const reply = Reply.fromBinary(transport(Call.toBinary({ method, request })))
  return reply.error ? undefined : (reply.response ?? new Uint8Array())
}

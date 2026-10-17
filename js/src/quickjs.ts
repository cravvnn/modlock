// The library as the QuickJS runtime loads it. The runtime installs its
// bridge as __modlock before the bundle runs and calls the bridge's event for
// each Call the host delivers.

import { bind, deliver, type Dispatch, type Transport } from './host.js'

export * from './index.js'

/** Bridge is what the QuickJS runtime installs as __modlock. */
interface Bridge {
  /** hostCall exchanges an encoded Call for the host's encoded Reply. */
  hostCall: Transport
  /** event is set by the library to answer each Call the host delivers. */
  event?: Dispatch
}

const bridge = (globalThis as unknown as { __modlock: Bridge }).__modlock
bind((call) => bridge.hostCall(call))
bridge.event = deliver

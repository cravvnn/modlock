// The test mod logs through the host on Start and throws on any other event,
// so one module exercises both host calls and a panic.
declare function host_call(data: Uint8Array): number
declare function host_read(data: Uint8Array): void

// hello is the encoded Call of Log with the message "hello from scriptc".
const hello = new Uint8Array([
  0x0a, 0x03, 0x4c, 0x6f, 0x67, 0x12, 0x14, 0x0a, 0x12, 0x68, 0x65, 0x6c, 0x6c,
  0x6f, 0x20, 0x66, 0x72, 0x6f, 0x6d, 0x20, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74,
  0x63,
])

// event answers the Call the host holds, size bytes long, with an empty Reply.
export function event(size: number): Uint8Array {
  // Read the Call from the host.
  const call = new Uint8Array(size)
  host_read(call)

  // A Call opens with its method's tag and length; only Start has five bytes.
  if (call[1] !== 5) {
    throw new Error('the test mod answers only Start')
  }

  // Log the greeting, read the host's reply, and answer.
  host_read(new Uint8Array(host_call(hello)))
  return new Uint8Array(0)
}

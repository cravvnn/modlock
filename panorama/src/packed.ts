// Packed bytes carry binary protobuf through a page title one character per
// byte: characters 1 to 127 are themselves, U+0100 to U+01FF are the byte plus
// 0x100. The controller's panorama.AppendPacked keeps only printable ASCII as
// itself, so a title holds no whitespace the browser could fold.

/**
 * unpackBytes reads the bytes packed in text, or null when any character is
 * outside the packing.
 */
export function unpackBytes(text: string): Uint8Array | null {
  const bytes = new Uint8Array(text.length)
  for (let index = 0; index < text.length; index++) {
    const code = text.charCodeAt(index)
    if (code >= 1 && code <= 127) bytes[index] = code
    else if (code >= 0x100 && code <= 0x1ff) bytes[index] = code - 0x100
    else return null
  }
  return bytes
}

/**
 * decodeUtf8 reads UTF-8 without the browser's TextDecoder, which Panorama
 * lacks. The engine's escape and URI decoder read the bytes natively, far
 * faster in Panorama than decoding each byte in script; malformed bytes throw.
 */
export function decodeUtf8(bytes: Uint8Array): string {
  // Turn the bytes into characters in slices the call's arguments can hold.
  let text = ''
  for (let at = 0; at < bytes.length; at += 4096)
    text += String.fromCharCode.apply(
      null,
      bytes.subarray(at, at + 4096) as unknown as number[],
    )
  return decodeURIComponent(escape(text))
}

// Generated protobuf readers decode each string with a TextDecoder.
if (typeof globalThis.TextDecoder === 'undefined') {
  globalThis.TextDecoder = class {
    decode(input = new Uint8Array()): string {
      return decodeUtf8(input)
    }
  } as unknown as typeof TextDecoder
}

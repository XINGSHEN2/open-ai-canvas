import { sha256 } from "@noble/hashes/sha256";

/** Identical SHA-256 on HTTPS and trusted LAN HTTP, including typed-array slices. */
export async function sha256Digest(data: BufferSource): Promise<ArrayBuffer> {
    if (globalThis.crypto?.subtle) return globalThis.crypto.subtle.digest("SHA-256", data);
    const bytes = ArrayBuffer.isView(data)
        ? new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
        : new Uint8Array(data);
    return new Uint8Array(sha256(bytes)).buffer;
}

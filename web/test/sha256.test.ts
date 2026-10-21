import { test, expect } from "bun:test";
import { createHash } from "node:crypto";
import { sha256Digest } from "../src/lib/sha256";

test("HTTP fallback matches SHA-256 for text, image bytes and array slices", async () => {
    const descriptor = Object.getOwnPropertyDescriptor(globalThis, "crypto");
    Object.defineProperty(globalThis, "crypto", { value: {}, configurable: true });
    try {
        for (const bytes of [
            new Uint8Array(),
            new TextEncoder().encode("abc"),
            new TextEncoder().encode("一辆车"),
            new Uint8Array([0, 1, 2, 3, 255]).subarray(1, 4),
            new Uint8Array(1024 * 1024).fill(173),
        ]) {
            const expected = createHash("sha256").update(bytes).digest("hex");
            expect(Buffer.from(await sha256Digest(bytes)).toString("hex")).toBe(expected);
        }
    } finally {
        if (descriptor) Object.defineProperty(globalThis, "crypto", descriptor);
    }
});

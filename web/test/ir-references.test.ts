import { describe, expect, test } from "bun:test";
import { prepareIRReferences } from "../src/lib/ir-references";
import type { CanvasResourceReference } from "../src/lib/canvas/canvas-resource-references";

const image = (id: string): CanvasResourceReference => ({ id, nodeId: id, kind: "image", label: id, title: id, active: true, storageKey: `resource:${id}` });
describe("IR reference contract", () => {
    test("uses actual media order, not mention order", () => {
        const result = prepareIRReferences("@b then @a", [image("a"), image("b")]);
        expect(result.prompt).toBe("<Image 2> then <Image 1>");
        expect(result.assets.map(a => a.resourceId)).toEqual(["a", "b"]);
    });
    test("separate numbering for audio and images", () => {
        const result = prepareIRReferences("@a @sound", [image("a"), { ...image("sound"), kind: "audio" }]);
        expect(result.prompt).toBe("<Image 1> <Audio 1>");
    });
    test("does not silently omit inaccessible media", () => {
        expect(() => prepareIRReferences("test", [{ ...image("a"), storageKey: undefined }])).toThrow("尚未同步");
    });
    test("expands selected text and ignores inactive references", () => {
        const result = prepareIRReferences("@script", [{ ...image("script"), kind: "text", text: "story" }, { ...image("a"), active: false }]);
        expect(result.assets).toHaveLength(0);
        expect(result.prompt).toContain("story");
    });
});

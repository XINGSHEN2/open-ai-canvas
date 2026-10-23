import { describe, expect, test } from "bun:test";
import { Window } from "happy-dom";
import { bindIRPrompt, compileIRPrompt, displayIRPrompt, irMentionReferences } from "../src/lib/ir-reference-bindings";
import { prepareIRReferences } from "../src/lib/ir-references";
import { renderEditableContent, serializeEditableValue, splitMentionText } from "../src/components/canvas/canvas-mention-editable";
import type { CanvasResourceReference } from "../src/lib/canvas/canvas-resource-references";

const assets = [
    { kind: "video" as const, resourceId: "motion" },
    { kind: "image" as const, resourceId: "woman" },
    { kind: "audio" as const, resourceId: "music" },
    { kind: "image" as const, resourceId: "room" },
];
const refs: CanvasResourceReference[] = assets.map((asset) => ({ id: asset.resourceId, nodeId: asset.resourceId, kind: asset.kind, storageKey: `resource:${asset.resourceId}`, active: true, label: asset.resourceId, title: asset.resourceId }));

describe("IR resource binding through application, editing and submission", () => {
    test("binds Picture/Image and case/spacing variants against independent per-media slots", () => {
        const bound = bindIRPrompt("<Picture 1>与<image2>，参考< VIDEO 1 >及<audio1>，<Subject 1>不变", assets);
        expect(bound).toBe("@[ir:image:woman]与@[ir:image:room]，参考@[ir:video:motion]及@[ir:audio:music]，<Subject 1>不变");
        expect(compileIRPrompt(bound, assets)).toBe("<Picture 1>与<Picture 2>，参考<Video 1>及<Audio 1>，<Subject 1>不变");
    });

    test("reorder compiles to new slots; deletion or replacement must fail, not silently point to another file", () => {
        const bound = bindIRPrompt("<Picture 1> walks in <Picture 2>", assets);
        const reversed = [...assets].reverse();
        expect(compileIRPrompt(bound, reversed)).toBe("<Picture 2> walks in <Picture 1>");
        expect(() => compileIRPrompt(bound, assets.filter((asset) => asset.resourceId !== "woman"))).toThrow("已移除或替换");
        expect(() => compileIRPrompt(bound, assets.map((asset) => asset.resourceId === "woman" ? { ...asset, resourceId: "another-woman" } : asset))).toThrow("已移除或替换");
        expect(prepareIRReferences(bound, [...refs].reverse()).prompt).toBe("<Image 2> walks in <Image 1>");
    });

    test("rejects stale apply, out-of-range and malformed model references", () => {
        expect(() => bindIRPrompt("<Image 1>", assets, "old-fingerprint", "new-fingerprint")).toThrow("已经变化");
        for (const prompt of ["<Image 3>", "<Video 2>", "<Audio 2>", "<Picture 0>", "<image-1>", "<Image abc>"]) {
            expect(() => bindIRPrompt(prompt, assets)).toThrow("超出当前");
            expect(() => compileIRPrompt(prompt, assets)).toThrow("超出当前");
        }
        expect(() => bindIRPrompt("@[asset:missing]", assets)).toThrow("未解析");
        expect(() => compileIRPrompt("@[ir:image:woman", assets)).toThrow("未解析");
        expect(() => compileIRPrompt("@[ir:image:%ZZ]", assets)).toThrow("格式无效");
    });

    test("renders friendly real mention chips and preserves stable tokens through serialization", async () => {
        const browser = new Window();
        const previousDocument = globalThis.document;
        const previousNode = globalThis.Node;
        const previousHTMLElement = globalThis.HTMLElement;
        Object.assign(globalThis, { document: browser.document, Node: browser.Node, HTMLElement: browser.HTMLElement });
        try {
            const prompt = bindIRPrompt("<Image 1>站在<Image 2>里面", assets);
            const references = irMentionReferences(prompt, refs);
            const editor = document.createElement("div");
            renderEditableContent(editor, prompt, references);
            expect(editor.textContent).toContain("图片1 · woman");
            expect(editor.textContent).not.toContain("@[ir:");
            expect(serializeEditableValue(editor)).toBe(prompt);
            expect(splitMentionText("@图片1 · woman", references).every((part) => part.type === "text")).toBe(true);
            const missingReferences = irMentionReferences(prompt, refs.filter((ref) => ref.id !== "woman"));
            renderEditableContent(editor, prompt, missingReferences);
            expect(editor.textContent).toContain("已移除的图片");
            expect(serializeEditableValue(editor)).toBe(prompt);
            expect(displayIRPrompt(prompt, assets)).toBe("<Image 1>站在<Image 2>里面");
        } finally {
            Object.assign(globalThis, { document: previousDocument, Node: previousNode, HTMLElement: previousHTMLElement });
            await browser.happyDOM.close();
        }
    });
});

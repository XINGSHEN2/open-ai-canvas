import { describe, expect, test } from "bun:test";
import { prepareBackendGenerationTask, submitBackendGenerationTask } from "../src/services/api/generation-task";
import { createModelChannel, defaultConfig, encodeChannelModel } from "../src/stores/use-config-store";
import { bindIRPrompt } from "../src/lib/ir-reference-bindings";
import { prepareIRReferences } from "../src/lib/ir-references";
import { buildCreationMentionReferences, displayCreationPrompt, expandCreationPrompt, removeCreationReferenceTokens, replaceCreationAttachmentReference } from "../src/pages/create/creation-references";
import { buildNodeGenerationContext } from "../src/components/canvas/canvas-node-generation";
import { buildShotAssetReferenceContext, resolveShotAssetMentionPrompt, type ShotAssetReferenceContext } from "../src/pages/projects/detail/workflow-shot-references";
import { applyCanvasConnectionPromptSync } from "../src/lib/canvas/canvas-resource-references";
import type { ProjectDetail } from "../src/services/api/projects";
import { CanvasNodeType, type CanvasNodeData } from "../src/types/canvas";
import type { ReferenceImage } from "../src/types/image";

function config() {
    const channel = createModelChannel({ id: "h3", name: "H3", interfaceType: "openai-video", models: ["MiniMax-H3"] });
    const model = encodeChannelModel(channel.id, "MiniMax-H3");
    return { ...defaultConfig, channels: [channel], model, videoModel: model, interfaceType: channel.interfaceType };
}
const image = (id: string): ReferenceImage & { previewUrl: string } => ({ id, name: id, type: "image/png", dataUrl: "", previewUrl: "", storageKey: `resource:${id}` });
const images = [image("woman"), image("room")];
const connections = (items: ReferenceImage[]) => items.map((item) => ({ id: item.id, fromNodeId: item.id, toNodeId: "target" }));
function canvasNodes(prompt: string, items: ReferenceImage[]): CanvasNodeData[] {
    return [
        ...items.map((item) => ({ id: item.id, type: CanvasNodeType.Image, title: item.id, position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { storageKey: item.storageKey } })),
        { id: "target", type: CanvasNodeType.Video, title: "target", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { composerContent: prompt } },
    ];
}

describe("all video entrances share final IR validation", () => {
    const bound = bindIRPrompt("<Picture 1> stands in <Picture 2>", images.map((item) => ({ kind: "image", resourceId: item.id })));
    test("creation, canvas, and shot compilers preserve bindings until the final submitted media order", async () => {
        const refs = buildCreationMentionReferences([], images);
        const creationPrompt = expandCreationPrompt(bound, refs, images);
        const canvasNodes: CanvasNodeData[] = [
            ...images.map((item) => ({ id: item.id, type: CanvasNodeType.Image, title: item.id, position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { storageKey: item.storageKey } })),
            { id: "target", type: CanvasNodeType.Video, title: "target", position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { composerContent: bound } },
        ];
        const canvas = buildNodeGenerationContext("target", canvasNodes, images.map((item) => ({ id: item.id, fromNodeId: item.id, toNodeId: "target" })), `${bound}；@[node:woman]`, []);
        // An ordinary @ mention must not cause a second IR-bound reference to be dropped.
        expect(canvas.referenceImages.map((item) => item.id)).toEqual(["woman", "room"]);
        const shotContext: ShotAssetReferenceContext = { mentionReferences: [], referenceImages: images, referenceVideos: [], referenceAudios: [], assetReferences: [], resolvedCharacterVersions: [] };
        const shotPrompt = resolveShotAssetMentionPrompt(bound, shotContext);
        for (const prompt of [creationPrompt, canvas.prompt, shotPrompt]) {
            const input = await prepareBackendGenerationTask({ mode: "video", prompt, config: config(), referenceImages: [...images].reverse() });
            expect(input.prompt).toContain("<Picture 2> stands in <Picture 1>");
            expect(input.prompt).not.toContain("@[ir:");
            expect((input.input as { prompt: string }).prompt).toBe(input.prompt);
        }
        expect(prepareIRReferences(bound, refs).prompt).toBe("<Image 1> stands in <Image 2>");
    });

    test("invalid final input is rejected before creating a billable backend task", async () => {
        let creates = 0;
        const dependencies = { createId: () => "test", createTask: async () => { creates++; throw new Error("should not submit"); }, waitTask: async () => { throw new Error("should not wait"); } };
        for (const prompt of [bound, "<Picture 2>", "@[asset:unresolved]"]) {
            await expect(submitBackendGenerationTask({ mode: "video", prompt, config: config(), referenceImages: [images[1]] }, dependencies)).rejects.toThrow();
        }
        expect(creates).toBe(0);
    });

    test("each real entrance preserves IR bindings through reference edits and rejects missing original media", async () => {
        const oldRefs = buildCreationMentionReferences([], images);
        const replaced = replaceCreationAttachmentReference(bound, images, "woman", image("replacement"));
        const changes = [
            { items: [...images].reverse(), prompt: bound, valid: true },
            { items: [images[1]], prompt: removeCreationReferenceTokens(bound, [oldRefs[0]]), valid: false },
            { items: replaced.attachments, prompt: replaced.prompt, valid: false },
        ];
        for (const change of changes) {
            const currentImages = change.items as typeof images;
            expect(change.prompt).toBe(bound);
            const creation = { prompt: expandCreationPrompt(change.prompt, buildCreationMentionReferences([], currentImages), currentImages), referenceImages: currentImages };
            const synced = applyCanvasConnectionPromptSync(canvasNodes(bound, images), connections(images), canvasNodes(bound, currentImages), connections(currentImages));
            const canvas = buildNodeGenerationContext("target", synced, connections(currentImages), synced.find((node) => node.id === "target")!.metadata!.composerContent!, []);
            const projectAssets = currentImages.map((item) => ({ id: item.id, title: item.name, category: "other", mediaType: "image", primaryVersionId: `version-${item.id}`, storageKey: item.storageKey }));
            const shotContext = buildShotAssetReferenceContext({ assets: projectAssets, shotReferences: projectAssets.map((asset) => ({ shotId: "shot", assetVersionId: asset.primaryVersionId, status: "linked" })) } as ProjectDetail, "shot");
            const shot = { prompt: resolveShotAssetMentionPrompt(bound, shotContext), referenceImages: shotContext.referenceImages };
            for (const entry of [creation, canvas, shot]) {
                const request = prepareBackendGenerationTask({ ...entry, mode: "video", config: config() });
                if (change.valid) expect((await request).prompt).toContain("<Picture 2> stands in <Picture 1>");
                else await expect(request).rejects.toThrow("已移除或替换");
            }
        }
        expect(displayCreationPrompt(bound, oldRefs)).toBe("<Image 1> stands in <Image 2>");
    });

    test("historic/manual numeric prompts validate against current arrays without guessing an old resource identity", async () => {
        const input = await prepareBackendGenerationTask({ mode: "video", prompt: "<Image1> in frame", config: config(), referenceImages: [images[1]] });
        expect(input.prompt).toBe("<Picture 1> in frame");
        expect((input.input as { referenceImages: ReferenceImage[] }).referenceImages[0].storageKey).toBe("resource:room");
        await expect(prepareBackendGenerationTask({ mode: "video", prompt: "<Image2>", config: config(), referenceImages: [images[1]] })).rejects.toThrow("超出当前");
    });

    test("a video used as its own IR reference remains selected when there are no input edges", async () => {
        const prompt = bindIRPrompt("参考<Video 1>", [{ kind: "video", resourceId: "original-video" }]);
        const target = canvasNodes(prompt, []).find((node) => node.id === "target")!;
        target.metadata = { ...target.metadata, storageKey: "resource:original-video" };
        const context = buildNodeGenerationContext("target", [target], [], prompt, []);
        expect(context.referenceVideos.map((item) => item.storageKey)).toEqual(["resource:original-video"]);
        const input = await prepareBackendGenerationTask({ ...context, mode: "video", config: config() });
        expect(input.prompt).toBe("参考<Video 1>");
    });

    test("literal numbered image references in non-video tasks are unchanged", async () => {
        const input = await prepareBackendGenerationTask({ mode: "image", prompt: "<Picture 99>", config: config() });
        expect(input.prompt).toBe("<Picture 99>");
    });

    test("switching away from video never submits internal IR resource tokens", async () => {
        let creates = 0;
        const dependencies = { createId: () => "test", createTask: async () => { creates++; throw new Error("should not submit"); }, waitTask: async () => { throw new Error("should not wait"); } };
        for (const mode of ["image", "text", "audio"] as const) {
            await expect(prepareBackendGenerationTask({ mode, prompt: bound, config: config(), referenceImages: images })).rejects.toThrow("请切回视频模式");
            await expect(submitBackendGenerationTask({ mode, prompt: bound, config: config(), referenceImages: images }, dependencies)).rejects.toThrow("请切回视频模式");
        }
        expect(creates).toBe(0);
    });

    test("text history uses descriptive placeholders, never current media or internal resource IDs", async () => {
        const textHistory = [{ role: "user" as const, content: bound }, { role: "assistant" as const, content: "完成" }];
        const input = await prepareBackendGenerationTask({ mode: "text", prompt: "总结一下", config: config(), referenceImages: [image("new-unrelated")], textHistory });
        const history = (input.input as { textHistory: typeof textHistory }).textHistory;
        expect(history[0].content).toBe("【历史图片引用，未附带素材】 stands in 【历史图片引用，未附带素材】");
        expect(history[1].content).toBe("完成");
        expect(JSON.stringify(history)).not.toContain("@[ir:");
        expect(JSON.stringify(history)).not.toContain("new-unrelated");
        expect(textHistory[0].content).toBe(bound);
        await expect(prepareBackendGenerationTask({ mode: "text", prompt: "总结一下", config: config(), textHistory: [{ role: "user", content: "@[ir:broken]" }] })).rejects.toThrow("历史消息含有无效 IR 引用");
    });
});

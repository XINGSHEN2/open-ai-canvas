import { resourceIdFromStorageKey } from "@/services/api/resources";
import type { CanvasResourceReference } from "@/lib/canvas/canvas-resource-references";
import type { IRAsset } from "@/services/api/context-ir";

type BindingAsset = Pick<IRAsset, "kind" | "resourceId">;
type MediaKind = BindingAsset["kind"];
const names = { image: "图片", video: "视频", audio: "音频" };
const resourcePattern = /@\[ir:(image|video|audio):([^\]\r\n]+)\]/g;
const numberedPattern = /<\s*(Picture|Image|Video|Audio)(?=\s|[-+\d>])\s*([^<>]*?)\s*>/gi;

function slots(assets: BindingAsset[]) {
    return {
        image: assets.filter((asset) => asset.kind === "image"),
        video: assets.filter((asset) => asset.kind === "video"),
        audio: assets.filter((asset) => asset.kind === "audio"),
    };
}

function mediaKind(name: string): MediaKind {
    return /^(image|picture)$/i.test(name) ? "image" : name.toLowerCase() as MediaKind;
}

function numberedLabel(kind: MediaKind, index: number, imageLabel: "Image" | "Picture" = "Image") {
    return `<${kind === "image" ? imageLabel : kind === "video" ? "Video" : "Audio"} ${index}>`;
}

function resourceToken(kind: MediaKind, resourceId: string) {
    return `@[ir:${kind}:${encodeURIComponent(resourceId)}]`;
}

function resourceId(value: string) {
    try { return decodeURIComponent(value); } catch { throw new Error("IR 素材引用格式无效，请重新优化提示词"); }
}

function replaceNumberedReferences(prompt: string, assets: BindingAsset[], replace: (kind: MediaKind, index: number, asset: BindingAsset, token: string) => string) {
    const byKind = slots(assets);
    return prompt.replace(numberedPattern, (token, name: string, rawIndex: string) => {
        const kind = mediaKind(name);
        const index = /^\d+$/.test(rawIndex) ? Number(rawIndex) : 0;
        const asset = byKind[kind][index - 1];
        if (!Number.isSafeInteger(index) || index < 1 || !asset) throw new Error(`提示词引用 ${token} 超出当前${names[kind]}范围（共 ${byKind[kind].length} 个），请检查素材或重新 IR 优化`);
        return replace(kind, index, asset, token);
    });
}

/** Bind the reviewed IR result to the exact resources used by that request, not mutable slots. */
export function bindIRPrompt(prompt: string, assets: BindingAsset[], savedFingerprint?: string, currentFingerprint?: string) {
    if (savedFingerprint !== currentFingerprint) throw new Error("提示词、素材或设置已经变化，请重新 IR 优化");
    const bound = replaceNumberedReferences(prompt, assets, (kind, _index, asset) => {
        if (!asset.resourceId) throw new Error("IR 素材尚未保存，请重新上传后优化");
        return resourceToken(kind, asset.resourceId);
    });
    compileIRPrompt(bound, assets);
    return bound;
}

/** The final submitted media arrays are authoritative. A removed/replaced resource is never remapped. */
export function compileIRPrompt(prompt: string, assets: BindingAsset[], imageLabel: "Image" | "Picture" = "Picture") {
    const byKind = slots(assets);
    const compiled = prompt.replace(resourcePattern, (_token, kind: MediaKind, encodedId: string) => {
        const id = resourceId(encodedId);
        const index = byKind[kind].findIndex((asset) => asset.resourceId === id);
        if (index < 0) throw new Error(`IR 引用的${names[kind]}已移除或替换，请重新 IR 优化，或删除该引用后重新选择素材`);
        return numberedLabel(kind, index + 1, imageLabel);
    });
    if (/@\[(?:ir|asset|node|attachment):/.test(compiled)) throw new Error("提示词含有未解析的素材引用，请重新选择素材或重新 IR 优化");
    return replaceNumberedReferences(compiled, assets, (kind, index) => numberedLabel(kind, index, imageLabel));
}

export function hasIRResourceReferences(prompt: string) {
    return prompt.includes("@[ir:");
}

/** Historical text has no attachment snapshot; never bind it to the current files. */
export function irPromptForTextHistory(prompt: string) {
    const text = prompt.replace(resourcePattern, (_token, kind: MediaKind) => `【历史${names[kind]}引用，未附带素材】`);
    if (hasIRResourceReferences(text)) throw new Error("历史消息含有无效 IR 引用，请新建对话后继续");
    return text;
}

/** Render-only references must never enter @ menus or rebind ordinary @ labels. */
export function irMentionReferences(prompt: string, references: CanvasResourceReference[]): CanvasResourceReference[] {
    const counts = { image: 0, video: 0, audio: 0 };
    const byResource = new Map<string, CanvasResourceReference>();
    for (const reference of references) {
        if (!reference.active || !["image", "character", "video", "audio"].includes(reference.kind)) continue;
        const kind: MediaKind = reference.kind === "character" ? "image" : reference.kind as MediaKind;
        const index = ++counts[kind];
        const id = resourceIdFromStorageKey(reference.storageKey);
        if (id && !byResource.has(resourceToken(kind, id))) byResource.set(resourceToken(kind, id), { ...reference, label: `${names[kind]}${index} · ${reference.title || reference.label}` });
    }
    return [...new Set(Array.from(prompt.matchAll(resourcePattern), (match) => match[0]))].map((token) => {
        const kind = token.split(":")[1] as MediaKind;
        const reference = byResource.get(token);
        return { ...reference, id: token, nodeId: "", assetId: undefined, kind, label: reference?.label || `已移除的${names[kind]}`, title: reference?.title || "请重新优化或删除此引用", mentionToken: token, active: true };
    });
}

export function displayIRPrompt(prompt: string, assets: BindingAsset[] = []) {
    const byKind = slots(assets);
    return prompt.replace(resourcePattern, (_token, kind: MediaKind, encodedId: string) => {
        let id: string;
        try { id = resourceId(encodedId); } catch { return "【无效素材引用】"; }
        const index = byKind[kind].findIndex((asset) => asset.resourceId === id);
        return index < 0 ? `【已绑定的${names[kind]}】` : numberedLabel(kind, index + 1);
    });
}

import { resourceIdFromStorageKey } from "@/services/api/resources";
import { canvasResourceMentionToken, type CanvasResourceReference } from "@/lib/canvas/canvas-resource-references";
import type { IRAsset } from "@/services/api/context-ir";

// Preserve per-media input order; never silently optimize without a selected reference.
export function prepareIRReferences(prompt: string, references: CanvasResourceReference[]) {
    const assets: IRAsset[] = [];
    const counts = { image: 0, video: 0, audio: 0 };
    let text = prompt;
    for (const reference of references) {
        if (!reference.active || reference.kind === "skill" || reference.kind === "tool") continue;
        const token = canvasResourceMentionToken(reference);
        if (reference.kind === "text") {
            if (text.includes(token) || text.includes(`@${reference.label}`)) {
                text = text.split(token).join(`【${reference.label}】`).split(`@${reference.label}`).join(`【${reference.label}】`);
                text += `\n\n【${reference.label}】\n${reference.text || ""}`;
            }
            continue;
        }
        if (reference.kind !== "image" && reference.kind !== "video" && reference.kind !== "audio") throw new Error("此参考类型暂不支持 IR，请使用图片、视频或音频素材");
        const resourceId = resourceIdFromStorageKey(reference.storageKey);
        if (!resourceId) throw new Error(`参考素材「${reference.label}」尚未同步到服务器，请先保存或重新上传素材`);
        const kind = reference.kind;
        const label = `<${kind === "image" ? "Image" : kind === "video" ? "Video" : "Audio"} ${++counts[kind]}>`;
        text = text.split(token).join(label).split(`@${reference.label}`).join(label);
        assets.push({ resourceId, kind, label: `${label} ${reference.title || reference.label}` });
    }
    return { prompt: text, assets };
}

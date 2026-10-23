import { resourceIdFromStorageKey } from "@/services/api/resources";
import { canvasResourceMentionToken, type CanvasResourceReference } from "@/lib/canvas/canvas-resource-references";
import type { IRAsset } from "@/services/api/context-ir";
import { compileIRPrompt } from "@/lib/ir-reference-bindings";

// Preserve per-media input order; never silently optimize without a selected reference.
export function prepareIRReferences(prompt: string, references: CanvasResourceReference[]) {
    const assets: IRAsset[] = [];
    const counts = { image: 0, video: 0, audio: 0 };
    const replacements = new Map<string, string>();
    const aliases = new Map<string, string | null>();
    const textReferences = new Map<string, string>();
    const addReplacement = (reference: CanvasResourceReference, label: string) => {
        const entries = [{ token: canvasResourceMentionToken(reference), label: reference.label }, ...(reference.mentionAliases || [])];
        for (const entry of entries) {
            const alias = `@${entry.label}`;
            if (entry.token !== alias) replacements.set(entry.token, label);
            if (entry.label) aliases.set(alias, aliases.has(alias) && aliases.get(alias) !== label ? null : label);
        }
    };
    for (const reference of references) {
        if (!reference.active || reference.kind === "skill" || reference.kind === "tool") continue;
        if (reference.kind === "text") {
            const label = `【${reference.label}】`;
            addReplacement(reference, label);
            textReferences.set(label, reference.text || "");
            continue;
        }
        if (reference.kind !== "image" && reference.kind !== "character" && reference.kind !== "video" && reference.kind !== "audio") throw new Error("此参考类型暂不支持 IR，请使用图片、视频或音频素材");
        const resourceId = resourceIdFromStorageKey(reference.storageKey);
        if (!resourceId) throw new Error(`参考素材「${reference.label}」尚未同步到服务器，请先保存或重新上传素材`);
        const kind = reference.kind === "character" ? "image" : reference.kind;
        const label = `<${kind === "image" ? "Image" : kind === "video" ? "Video" : "Audio"} ${++counts[kind]}>`;
        addReplacement(reference, label);
        assets.push({ resourceId, kind, label: `${label} ${reference.title || reference.label}` });
    }
    if (counts.image > 8 || counts.video > 3 || counts.audio > 3) throw new Error("IR 最多支持 8 张图片、3 个视频和 3 个音频；请减少参考素材");
    const escape = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const keys = [...new Set([...replacements.keys(), ...aliases.keys()])].sort((a, b) => b.length - a.length);
    const usedText = new Set<string>();
    // One pass, longest token first: @图10 must not be partly consumed as @图1.
    const pattern = keys.map((key) => escape(key) + (replacements.has(key) ? "" : "(?![\\p{L}\\p{N}_-])")).join("|");
    let text = pattern ? prompt.replace(new RegExp(pattern, "gu"), (token) => {
        const label = replacements.get(token) ?? aliases.get(token);
        if (!label) throw new Error(`素材名称「${token}」重复，请通过 @ 菜单重新选择素材`);
        if (textReferences.has(label)) usedText.add(label);
        return label;
    }) : prompt;
    for (const label of usedText) text += `\n\n${label}\n${textReferences.get(label)}`;
    return { prompt: compileIRPrompt(text, assets, "Image"), assets };
}

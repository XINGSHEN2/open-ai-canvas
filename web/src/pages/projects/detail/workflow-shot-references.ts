import { canvasResourceMentionToken, type CanvasResourceReference } from "@/lib/canvas/canvas-resource-references";
import { assetCategoryLabel, normalizeAssetCategory } from "@/lib/asset-category";
import { resourceFileUrl, resourceIdFromStorageKey, resourceStorageKey } from "@/services/api/resources";
import type { CharacterRepresentation, ProjectAsset, ProjectDetail, ShotAssetReference } from "@/services/api/projects";
import type { AssetCategory } from "@/stores/use-asset-store";
import type { ReferenceImage } from "@/types/image";
import type { ReferenceAudio, ReferenceVideo } from "@/types/media";

export type ShotAssetReferenceContext = {
    mentionReferences: CanvasResourceReference[];
    referenceImages: ReferenceImage[];
    referenceVideos: ReferenceVideo[];
    referenceAudios: ReferenceAudio[];
    assetReferences: ShotPromptAssetReference[];
    resolvedCharacterVersions: Array<{ assetId: string; versionId: string }>;
};

export type ShotPromptAssetReference = {
    assetId: string;
    title: string;
    category: string;
    imageIndex?: number;
    videoIndex?: number;
    audioIndex?: number;
    voiceDescription?: string;
    dialogue?: string;
};

export function buildShotAssetReferenceContext(detail: ProjectDetail, shotId: string): ShotAssetReferenceContext {
    const assetByVersionId = new Map(detail.assets.filter((asset) => asset.primaryVersionId).map((asset) => [asset.primaryVersionId as string, asset]));
    const seenAssetIds = new Set<string>();
    const entries = (detail.shotReferences || []).flatMap((reference) => {
        if (reference.shotId !== shotId || reference.status !== "linked") return [];
        const asset = reference.asset || assetByVersionId.get(reference.assetVersionId);
        if (!asset || seenAssetIds.has(asset.id)) return [];
        const image = asset.mediaType === "video" || asset.mediaType === "audio" ? undefined : projectAssetReferenceImage(asset, reference);
        const media = projectAssetReferenceMedia(asset, reference);
        if (!image && !media) return [];
        seenAssetIds.add(asset.id);
        return [{ asset, image, media, reference }];
    });

    const referenceImages: ReferenceImage[] = [];
    const referenceVideos: ReferenceVideo[] = [];
    const referenceAudios: ReferenceAudio[] = [];
    const audioIndexByResourceId = new Map<string, number>();
    const mentionReferences: CanvasResourceReference[] = [];
    const addAudio = (audio: ReferenceAudio) => {
        const key = audio.storageKey || audio.id;
        let index = audioIndexByResourceId.get(key);
        if (!index) {
            index = referenceAudios.length + 1;
            referenceAudios.push(audio);
            audioIndexByResourceId.set(key, index);
        }
        return index;
    };
    const assetReferences = entries.map(({ asset, image, media }) => {
        let imageIndex: number | undefined;
        let videoIndex: number | undefined;
        let audioIndex: number | undefined;
        if (image) { referenceImages.push(image); imageIndex = referenceImages.length; }
        if (media && asset.mediaType === "video") { referenceVideos.push(media); videoIndex = referenceVideos.length; }
        if (media && asset.mediaType === "audio") audioIndex = addAudio(media);
        mentionReferences.push({
            id: `project-asset:${asset.id}`, nodeId: "", assetId: asset.id,
            kind: image ? (asset.character ? "character" : "image") : asset.mediaType === "video" ? "video" : "audio",
            label: asset.title, title: asset.title, previewUrl: (image || media)?.url,
            storageKey: (image || media)?.storageKey, active: true, category: projectAssetCategory(asset.category),
        });
        const sampleResourceId = stringValue(asset.character?.voice?.profile.sampleResourceId);
        if (sampleResourceId) {
            audioIndex = addAudio({
                    id: `project-character-voice:${asset.id}`,
                    name: `${asset.title}-声音样本`,
                    type: "audio/*",
                    url: resourceFileUrl(sampleResourceId),
                    storageKey: resourceStorageKey(sampleResourceId),
            });
        }
        return {
            assetId: asset.id,
            title: asset.title,
            category: asset.category,
            ...(imageIndex ? { imageIndex } : {}),
            ...(videoIndex ? { videoIndex } : {}),
            ...(audioIndex ? { audioIndex } : {}),
            ...(asset.character ? { voiceDescription: characterVoiceDescription(asset) } : {}),
        } satisfies ShotPromptAssetReference;
    });

    return {
        mentionReferences,
        referenceImages,
        referenceVideos,
        referenceAudios,
        assetReferences,
        resolvedCharacterVersions: entries.flatMap(({ asset, reference }) => asset.character ? [{ assetId: asset.id, versionId: reference.referencedVersion?.id || reference.assetVersionId || asset.character.versionId }] : []),
    };
}

export function resolveShotAssetMentionPrompt(prompt: string, context: ShotAssetReferenceContext, options: { dialogue?: string } = {}) {
    const imageLabelByAssetId = new Map(context.assetReferences.map((reference) => [reference.assetId,
        reference.imageIndex ? `图片${reference.imageIndex}` : reference.videoIndex ? `视频${reference.videoIndex}` : `音频${reference.audioIndex}`]));
    const unresolved = new Set<string>();
    const resolved = prompt.replace(/@\[asset:([^\]]+)\]/g, (token, assetId: string) => {
        const label = imageLabelByAssetId.get(assetId);
        if (!label) {
            unresolved.add(token);
            return token;
        }
        return label;
    });
    if (unresolved.size) throw new Error(`提示词中的 ${Array.from(unresolved).join("、")} 未绑定到当前镜头，请重新选择资产或删除引用`);
    const dialogueContext = withShotDialogue(context.assetReferences, options.dialogue);
    const assetBlock = compileShotAssetReferencePrompt(dialogueContext.references);
    const dialogueBlock = dialogueContext.unassignedDialogue ? `【镜头台词】\n\n${dialogueContext.unassignedDialogue}` : "";
    return [resolved.trim(), assetBlock, dialogueBlock].filter(Boolean).join("\n\n");
}

export function ensureShotAssetMentionPrompt(prompt: string, references: Array<Pick<CanvasResourceReference, "assetId" | "kind" | "label" | "title">>) {
    const value = prompt.trim();
    if (!value) return "";
    const mentionedAssetIds = new Set(Array.from(value.matchAll(/@\[asset:([^\]]+)\]/g), (match) => match[1]));
    const missing = references.filter((reference): reference is typeof reference & { assetId: string } => Boolean(reference.assetId && !mentionedAssetIds.has(reference.assetId)));
    if (!missing.length) return value;
    const block = (title: string, items: typeof missing) => items.length
        ? [title, ...items.map((reference) => `${reference.title || reference.label}：@[asset:${reference.assetId}]`)].join("\n")
        : "";
    const characters = missing.filter((reference) => reference.kind === "character");
    const otherAssets = missing.filter((reference) => reference.kind !== "character");
    return [value, block("【角色参考】", characters), block("【场景与道具参考】", otherAssets)].filter(Boolean).join("\n\n");
}

export function compileShotAssetReferencePrompt(references: ShotPromptAssetReference[]) {
    if (!references.length) return "";
    const lines = references.flatMap((reference) => {
        const visualLabel = reference.category === "character" ? "人物参考" : `${projectAssetCategoryLabel(reference.category)}参考`;
        const media = [reference.imageIndex ? `${visualLabel}：图片${reference.imageIndex}` : "", reference.videoIndex ? `视频参考：视频${reference.videoIndex}` : "", reference.audioIndex ? `声音参考：音频${reference.audioIndex}` : ""].filter(Boolean).join("；");
        return [
            `- ${reference.title}：${media}`,
            reference.voiceDescription ? `  声音画像：${reference.voiceDescription}` : "",
            reference.dialogue ? `  镜头台词：${reference.dialogue}` : "",
        ].filter(Boolean);
    });
    return ["【资产参考】", "", ...lines].join("\n");
}

// IR and video submission share exactly the same per-kind ordering, including voice samples.
export function shotIRReferences(context: ShotAssetReferenceContext): CanvasResourceReference[] {
    return (["image", "video", "audio"] as const).flatMap((kind) => {
        const media = kind === "image" ? context.referenceImages : kind === "video" ? context.referenceVideos : context.referenceAudios;
        return media.map((item) => {
            const matches = context.mentionReferences.filter((reference) => (reference.kind === "character" ? "image" : reference.kind) === kind && (reference.assetId === item.id || Boolean(kind === "audio" && item.storageKey && reference.storageKey === item.storageKey)));
            const mention = matches[0];
            return { ...mention, id: mention?.id || item.id, nodeId: "", kind, label: mention?.label || item.name, title: item.name, storageKey: item.storageKey, active: true,
                mentionAliases: matches.slice(1).map((reference) => ({ token: canvasResourceMentionToken(reference), label: reference.label })),
            };
        });
    });
}

function projectAssetReferenceMedia(asset: ProjectAsset, reference?: ShotAssetReference): ReferenceVideo | undefined {
    if (asset.mediaType !== "video" && asset.mediaType !== "audio") return undefined;
    const representation = reference?.referencedVersion?.representations.find((item) => item.mediaType.startsWith(asset.mediaType));
    const storageKey = representation ? resourceStorageKey(representation.resourceId) : asset.storageKey;
    if (!storageKey) return undefined;
    const resourceId = resourceIdFromStorageKey(storageKey);
    return { id: asset.id, name: asset.title, type: `${asset.mediaType}/*`, url: resourceId ? resourceFileUrl(resourceId) : "", storageKey, role: "reference" };
}

function projectAssetReferenceImage(asset: ProjectAsset, reference?: ShotAssetReference): ReferenceImage | undefined {
    const boundRepresentations = reference?.referencedVersion?.representations || [];
    const representation = preferredVisualRepresentation(boundRepresentations)
        || (asset.character ? preferredVisualRepresentation(asset.character.representations) : undefined);
    if (representation) {
        return {
            id: asset.id,
            name: asset.title,
            type: "image/*",
            dataUrl: "",
            url: resourceFileUrl(representation.resourceId),
            storageKey: resourceStorageKey(representation.resourceId),
        };
    }
    if (asset.mediaType !== "image" || !asset.storageKey) return undefined;
    const resourceId = resourceIdFromStorageKey(asset.storageKey);
    return {
        id: asset.id,
        name: asset.title,
        type: "image/*",
        dataUrl: "",
        ...(resourceId ? { url: resourceFileUrl(resourceId) } : {}),
        storageKey: asset.storageKey,
    };
}

function preferredVisualRepresentation(representations: CharacterRepresentation[]) {
    return representations.find((item) => item.role === "turnaround_sheet")
        || representations.find((item) => item.role === "primary")
        || representations.find((item) => item.role === "front")
        || representations.find((item) => item.mediaType.startsWith("image"));
}

function withShotDialogue(references: ShotPromptAssetReference[], dialogue?: string) {
    const value = stringValue(dialogue);
    if (!value) return { references, unassignedDialogue: "" };
    const characters = references.filter((reference) => reference.category === "character");
    if (characters.length === 1) {
        return {
            references: references.map((reference) => reference.assetId === characters[0]?.assetId ? { ...reference, dialogue: value } : reference),
            unassignedDialogue: "",
        };
    }
    const resolved = references.map((reference) => {
        if (reference.category !== "character") return reference;
        const matched = dialogueForCharacter(value, reference.title);
        return matched ? { ...reference, dialogue: matched } : reference;
    });
    return {
        references: resolved,
        unassignedDialogue: resolved.some((reference) => reference.dialogue) ? "" : value,
    };
}

function dialogueForCharacter(dialogue: string, characterName: string) {
    const escaped = characterName.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const pattern = new RegExp(`(?:^|\\n)\\s*${escaped}\\s*[：:]\\s*([^\\n]+)`, "g");
    return Array.from(dialogue.matchAll(pattern), (match) => match[1]?.trim()).filter(Boolean).join("；");
}

function projectAssetCategory(value: string): AssetCategory {
    return normalizeAssetCategory(value);
}

function projectAssetCategoryLabel(value: string) {
    return assetCategoryLabel(value);
}

function characterVoiceDescription(asset: ProjectAsset) {
    const definition = asset.character?.definition || {};
    const voice = asset.character?.voice;
    return [
        stringValue(definition.voiceLanguage) || stringValue(voice?.profile.language),
        stringValue(definition.voiceAge),
        stringValue(definition.voiceTimbre) || stringValue(voice?.profile.timbre),
        stringValue(voice?.instructions),
    ].filter(Boolean).join("；");
}

function stringValue(value: unknown) {
    return typeof value === "string" ? value.trim() : "";
}

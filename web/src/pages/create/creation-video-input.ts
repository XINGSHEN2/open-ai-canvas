import { modelCapabilityConfigFor } from "@/lib/model-capabilities";
import { modelCompatibilityError, type ModelRequirements } from "@/lib/model-selection";
import { resolveModelRequestConfig, type AiConfig } from "@/stores/use-config-store";

export function creationVideoInputState(config: AiConfig, model: string, requirements: ModelRequirements) {
    const profile = modelCapabilityConfigFor(config, model).video!;
    const input = requirements.input;
    const images = input?.imageCount || 0;
    const videos = input?.videoCount || 0;
    const audios = input?.audioCount || 0;
    const frames = resolveModelRequestConfig(config, model).interfaceType === "gitee-h3-video" && profile.operations.length === 1 && profile.operations[0] === "image_to_video";
    const referenceOnly = profile.operations.length === 1 && profile.operations[0] === "reference_to_video";
    const textOnly = profile.operations.length === 1 && profile.operations[0] === "text_to_video";
    const hint = frames ? "请添加两张图片：第 1 张为首帧，第 2 张为尾帧；可拖动素材调整顺序。"
        : textOnly ? "文生视频无需参考素材。要使用素材，请先切换为参考生视频或首尾帧模型。"
        : `添加参考素材：最多 ${profile.references.maxImages} 张图片、${profile.references.maxVideos} 个视频、${profile.references.maxAudios} 个音频。`;
    const error = frames && (images !== 2 || videos > 0 || audios > 0) ? "请提供恰好两张图片作为首帧和尾帧，并移除视频、音频。"
        : referenceOnly && images + videos + audios === 0 ? "请先添加至少一个参考素材。"
        : modelCompatibilityError(config, model, requirements);
    return { hint, error, frames };
}

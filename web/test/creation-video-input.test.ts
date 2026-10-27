import { expect, test } from "bun:test";
import { creationVideoInputState } from "../src/pages/create/creation-video-input";
import { defaultModelCapabilityConfig } from "../src/lib/model-capabilities";
import { modelCompatibilityError } from "../src/lib/model-selection";
import { defaultConfig, type AiConfig, type ModelChannel } from "../src/stores/use-config-store";

function fixture(operation: string, images: number) {
    const capabilityConfig = defaultModelCapabilityConfig(undefined, "h3");
    capabilityConfig.video!.operations = [operation];
    Object.assign(capabilityConfig.video!.references, { maxImages: images, maxVideos: 0, maxAudios: 0 });
    const channel: ModelChannel = { id: "gitee", name: "Gitee", baseUrl: "https://example.com", apiKey: "", apiFormat: "openai", interfaceType: "gitee-h3-video", models: ["h3"], modelCosts: [{ model: "h3", protocol: "gitee-h3-video", capability: "video", capabilityConfig }] };
    return { ...defaultConfig, channels: [channel], videoModels: ["gitee::h3"] } as AiConfig;
}
const request = (imageCount = 0, videoCount = 0) => ({ capability: "video" as const, input: { textCount: 1, imageCount, videoCount, audioCount: 0, characterCount: 0 } });

test("选模型不要求素材齐全，但参考生成提交必须有素材", () => {
    const config = fixture("reference_to_video", 9);
    expect(modelCompatibilityError(config, "gitee::h3", { capability: "video" })).toBe("");
    expect(creationVideoInputState(config, "gitee::h3", request()).error).toContain("至少一个");
    expect(creationVideoInputState(config, "gitee::h3", request(1)).error).toBe("");
});
test("首尾帧必须两图且无视频，提示顺序", () => {
    const config = fixture("image_to_video", 2);
    for (const n of [0, 1, 3]) expect(creationVideoInputState(config, "gitee::h3", request(n)).error).toContain("恰好两张");
    expect(creationVideoInputState(config, "gitee::h3", request(2)).error).toBe("");
    expect(creationVideoInputState(config, "gitee::h3", request(2, 1)).error).not.toBe("");
    expect(creationVideoInputState(config, "gitee::h3", request(2)).hint).toContain("第 1 张为首帧");
});
test("切换文生模式后保留的图片不得发送", () => {
    const config = fixture("text_to_video", 0);
    expect(creationVideoInputState(config, "gitee::h3", request()).error).toBe("");
    expect(creationVideoInputState(config, "gitee::h3", request(1)).error).not.toBe("");
});

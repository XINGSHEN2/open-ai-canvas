import { expect, test } from "bun:test";
import { defaultModelCapabilityConfig } from "../src/lib/model-capabilities";
import { modelQuoteRequest } from "../src/lib/model-pricing";
import { inferVideoOperation } from "../src/lib/model-selection";
import { creationVideoConfig } from "../src/pages/create/creation-generation-config";
import { prepareBackendGenerationTask } from "../src/services/api/generation-task";
import { defaultConfig, type AiConfig } from "../src/stores/use-config-store";

function videoConfig(generateAudio: { supported: boolean; default: boolean }): AiConfig {
    const capabilityConfig = defaultModelCapabilityConfig();
    capabilityConfig.video!.generateAudio = generateAudio;
    capabilityConfig.video!.watermark = { supported: false, default: false };
    return {
        ...defaultConfig,
        model: "gravity::minimax-h3-a",
        videoModel: "gravity::minimax-h3-a",
        videoGenerateAudio: "true",
        videoWatermark: "true",
        channels: [
            {
                id: "gravity",
                name: "万有引力",
                baseUrl: "/api",
                apiKey: "",
                apiFormat: "openai",
                scope: "system",
                models: ["minimax-h3-a"],
                modelCosts: [{ model: "minimax-h3-a", capability: "video", billingMode: "per_second", unitPriceMicrocredits: 1, capabilityConfig }],
            },
        ],
    };
}

test.each([1, 2, 4])("Ref2VA-only %s 张图片的报价和提交一致", async (imageCount) => {
    const config = videoConfig({ supported: false, default: false });
    const profile = config.channels[0]!.modelCosts![0]!.capabilityConfig!.video!;
    profile.operations = ["reference_to_video"];
    profile.defaultOperation = "reference_to_video";
    profile.references.maxImages = 9;
    const input = { textCount: 1, imageCount, characterCount: 0, videoCount: 0, audioCount: 0 };
    const operation = inferVideoOperation(input, profile.operations);
    expect(operation).toBe("reference_to_video");
    const requestConfig = creationVideoConfig(config, config.videoModel, { ratio: "16:9", seconds: "6", videoQuality: "480" });
    expect(modelQuoteRequest(requestConfig, requestConfig.model, "video", { capability: "video", input })?.intent.operation).toBe(operation);
    const referenceImages = Array.from({ length: imageCount }, (_, i) => ({ id: `ref-${i}`, name: `ref-${i}`, type: "image/png", dataUrl: "", url: `https://example.com/ref-${i}.png` }));
    const task = await prepareBackendGenerationTask({ mode: "video", prompt: "test", config: requestConfig, referenceImages, metadata: { videoEditOperation: operation } });
    expect(task.operation).toBe(operation);
    expect(task.input.metadata).toMatchObject({ videoEditOperation: operation });
});

test("仅参考模型的音频、视频和空输入不被误判为 I2V；普通模型行为不变", () => {
    const input = { textCount: 1, imageCount: 0, characterCount: 0, videoCount: 0, audioCount: 0 };
    expect(inferVideoOperation({ ...input, audioCount: 1 }, ["reference_to_video"])).toBe("reference_to_video");
    expect(inferVideoOperation({ ...input, videoCount: 1 }, ["reference_to_video"])).toBe("reference_to_video");
    expect(inferVideoOperation(input, ["reference_to_video"])).toBe("text_to_video");
    expect(inferVideoOperation({ ...input, imageCount: 1 }, ["image_to_video", "reference_to_video"])).toBe("image_to_video");
});

test("创作台无声视频报价、提交与重试不携带全局音频和水印默认值", async () => {
    const config = videoConfig({ supported: false, default: false });
    const requestConfig = creationVideoConfig(config, config.videoModel, { ratio: "16:9", seconds: "6", videoQuality: "480" });
    const quote = modelQuoteRequest(requestConfig, requestConfig.model, "video");
    expect(quote?.intent.options).toMatchObject({ size: "16:9", vquality: "480", videoSeconds: 6, videoGenerateAudio: false, videoWatermark: false });

    for (const retryOf of [undefined, "failed-task"]) {
        const task = await prepareBackendGenerationTask({ mode: "video", prompt: "小猫在流泪", config: requestConfig, retryOf });
        expect(task.input).toMatchObject({
            config: { size: "16:9", vquality: "480", videoSeconds: "6", videoGenerateAudio: "false", videoWatermark: "false" },
            ...(retryOf ? { metadata: { retryOf } } : {}),
        });
        expect(task.operation).toBe("text_to_video");
    }
    expect(config.videoGenerateAudio).toBe("true");
    expect(config.videoWatermark).toBe("true");
});

test.each([false, true])("创作台支持同步音频时遵循模型默认值 %s", (audioDefault) => {
    const config = videoConfig({ supported: true, default: audioDefault });
    config.videoGenerateAudio = String(!audioDefault);
    const requestConfig = creationVideoConfig(config, config.videoModel, { ratio: "16:9", seconds: "6", videoQuality: "480" });
    expect(requestConfig.videoGenerateAudio).toBe(String(audioDefault));
    expect(modelQuoteRequest(requestConfig, requestConfig.model, "video")?.intent.options?.videoGenerateAudio).toBe(audioDefault);
});

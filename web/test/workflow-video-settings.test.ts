import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";

const source = readFileSync(new URL("../src/pages/projects/detail/workflow-production-workbench.tsx", import.meta.url), "utf8");
test("preview offers only video and old production routes resolve to video", () => {
    expect(source).not.toContain('value: "storyboard", label: "分镜图"');
    expect(source).not.toContain('value: "previz", label: "动作预演"');
    const route = readFileSync(new URL("../src/pages/projects/detail/workflow.tsx", import.meta.url), "utf8");
    expect(route).toContain('stage !== "storyboard" && stage !== "previz"');
});
test("video settings hide skills, camera controls and advanced panel", () => {
    expect(source).toContain('generationCapability !== "video" ? <Form.Item label="技能库"');
    expect(source).toContain('generationCapability !== "video" ? <div className="workflow-settings-section">');
    expect(source).toContain('generationCapability !== "video" ? <WorkflowDisclosure');
    for (const label of ["生成模型", "镜头时长（秒）", "画幅", "分辨率"]) expect(source).toContain(`label="${label}"`);
});
test("hidden supplementary text and skills do not alter video requests", () => {
    expect(source).toContain('? values.videoPrompt');
    expect(source).toContain('dialogue: mode === "video" ? undefined : values.dialogue');
    expect(source).toContain('generationCapability !== "video" ? <div className="workflow-form-grid">');
    expect(source).toContain('mode === "video" ? { prompt: resolvedPrompt, metadata: {} } : await skillRuntime.prepare');
});
test("saving retains unmounted historical fields", () => {
    expect(source).toContain('saveShot.mutate({ ...form.getFieldsValue(true), ...values })');
    expect(source).toContain('const values = { ...form.getFieldsValue(true), ...validated } as ShotEditorValues');
});

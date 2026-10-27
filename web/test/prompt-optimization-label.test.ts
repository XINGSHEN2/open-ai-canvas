import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";

const source = (path: string) => readFileSync(new URL(`../src/${path}`, import.meta.url), "utf8");

test("all video entrances use the shared prompt optimization label", () => {
    for (const path of ["pages/create/creation-workspace.tsx", "components/canvas/canvas-node-prompt-panel.tsx", "pages/projects/detail/workflow-production-workbench.tsx"]) {
        expect(source(path)).toContain("<IRPromptButton");
    }
    const component = source("components/ir-prompt-button.tsx");
    expect(component).toContain('busy ? "提示词优化 · 处理中" : "提示词优化"');
    expect(component).toContain('<AppDrawer title="提示词优化"');
    expect(component).toContain('hasRequested ? "重新优化提示词" : "开始优化提示词"');
    expect(component).not.toContain('title="IR 优化"');
    expect(component).not.toContain('"发送 IR 优化请求"');
});

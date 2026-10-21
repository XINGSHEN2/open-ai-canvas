import { useEffect, useRef, useState } from "react";
import { Alert, Button, Input, Space, Typography } from "antd";
import { AppDrawer } from "@/components/ui/product/app-drawer/app-drawer";
import { createIRPrompt, queryIRPrompt, type IRPromptInput } from "@/services/api/context-ir";

type Props = { prompt: string; fingerprint: string; disabled?: boolean; prepare: () => Promise<IRPromptInput>; onApply: (text: string) => void };

/** Preview-only: applying never submits a video or changes generation settings. */
export function IRPromptButton({ prompt, fingerprint, disabled, prepare, onApply }: Props) {
    const [open, setOpen] = useState(false);
    const [busy, setBusy] = useState(false);
    const [draft, setDraft] = useState("");
    const [error, setError] = useState("");
    const [original, setOriginal] = useState("");
    const [snapshot, setSnapshot] = useState("");
    const [input, setInput] = useState<IRPromptInput | null>(null);
    const [restore, setRestore] = useState<{ original: string; applied: string } | null>(null);
    const controller = useRef<AbortController | null>(null);
    const lock = useRef(false);
    useEffect(() => () => controller.current?.abort(), []);
    const stale = snapshot !== fingerprint;
    async function start() {
        if (lock.current) return;
        lock.current = true;
        controller.current?.abort();
        const abort = new AbortController(); controller.current = abort;
        setOpen(true); setBusy(true); setDraft(""); setError(""); setOriginal(prompt); setSnapshot(fingerprint); setInput(null);
        try {
            const request = await prepare();
            if (abort.signal.aborted) return;
            setInput(request);
            let job = await createIRPrompt(request, abort.signal);
            const deadline = Date.now() + 31 * 60_000;
            while (job.status === "running") {
                if (Date.now() > deadline) throw new Error("等待 IR 超时，请稍后重试");
                await new Promise<void>((resolve, reject) => {
                    const cancel = () => { clearTimeout(timer); reject(new DOMException("已停止等待", "AbortError")); };
                    const timer = setTimeout(() => { abort.signal.removeEventListener("abort", cancel); resolve(); }, 2000);
                    abort.signal.addEventListener("abort", cancel, { once: true });
                });
                job = await queryIRPrompt(job.id, abort.signal);
            }
            if (job.status !== "completed" || !job.prompt) throw new Error(job.error || "IR 未返回提示词");
            setDraft(job.prompt);
        } catch (failure) {
            if (!abort.signal.aborted) setError(failure instanceof Error ? failure.message : "IR 优化失败");
        } finally { lock.current = false; if (!abort.signal.aborted) setBusy(false); }
    }
    return <>
        <Button size="small" disabled={disabled || !prompt.trim() || busy} onClick={() => void start()} title={!prompt.trim() ? "请先描述视频需求" : "结合参考素材优化提示词，不自动生成视频"}>IR 优化</Button>
        {restore && prompt === restore.applied ? <Button size="small" type="text" disabled={disabled || busy} onClick={() => { onApply(restore.original); setRestore(null); }}>恢复优化前</Button> : null}
        <AppDrawer title="IR 优化结果" open={open} onClose={() => setOpen(false)} size={640}>
            <div data-canvas-no-zoom data-canvas-wheel-scroll className="flex flex-col gap-4">
                <details><summary>原始需求</summary><Typography.Paragraph style={{ whiteSpace: "pre-wrap" }}>{original}</Typography.Paragraph></details>
                {busy ? <Alert type="info" showIcon title="正在分析素材并编写提示词…" description="关闭面板不会取消后台任务。请勿重复提交；处理完成后可重新打开结果。" /> : null}
                {error ? <Alert type="error" showIcon title={error} description="原提示词未修改，你仍可使用原提示词生成视频。" /> : null}
                {stale ? <Alert type="warning" showIcon title="提示词、素材或设置已经变化" description="此结果基于优化前的输入，请重新优化以避免引用错位。" /> : null}
                <label htmlFor="ir-prompt-draft">优化后的提示词（可编辑）</label>
                <Input.TextArea id="ir-prompt-draft" value={draft} onChange={(event) => setDraft(event.target.value)} disabled={busy} autoSize={{ minRows: 16, maxRows: 28 }} />
                {input ? <Typography.Text type="secondary">参考素材：{input.assets.map((asset) => asset.label).join("、") || "无"}；{input.seconds} 秒 · {input.ratio} · {input.resolution}P</Typography.Text> : null}
                <Typography.Text type="secondary">本次优化不会改变素材和生成设置。应用后，请手动点击生成视频。</Typography.Text>
                <Space><Button onClick={() => setOpen(false)}>关闭</Button><Button disabled={busy || disabled || !prompt.trim()} onClick={() => void start()}>重新优化</Button><Button type="primary" disabled={busy || disabled || stale || !draft.trim()} onClick={() => { setRestore({ original: prompt, applied: draft }); onApply(draft); setOpen(false); }}>应用到提示词</Button></Space>
            </div>
        </AppDrawer>
        {!open && (busy || draft || error) ? <Button size="small" type="text" onClick={() => setOpen(true)}>{busy ? "IR 处理中" : "查看 IR 结果"}</Button> : null}
    </>;
}

import { useEffect, useMemo, useRef, useState } from "react";
import { Alert, Button, Input, Space, Typography } from "antd";
import { AppDrawer } from "@/components/ui/product/app-drawer/app-drawer";
import { createIRPrompt, queryIRPrompt, type IRPromptInput, type IRPromptJob } from "@/services/api/context-ir";
import { localForageStorageForScope } from "@/lib/localforage-storage";
import { useUserStore } from "@/stores/use-user-store";
import { irDraftStorage, resumeIRPrompt, waitIRPoll, type IRPromptDraft } from "@/services/ir-prompt-session";
import { ApiError } from "@/services/api/request";
import { bindIRPrompt, displayIRPrompt } from "@/lib/ir-reference-bindings";

type Props = { persistenceKey: string; prompt: string; fingerprint: string; disabled?: boolean; prepare: () => Promise<IRPromptInput>; onApply: (text: string) => void };

/** Preview-only: applying never submits a video or changes generation settings. */
export function IRPromptButton({ persistenceKey, prompt, fingerprint, disabled, prepare, onApply }: Props) {
    const userId = useUserStore((state) => state.user?.id || "guest");
    const storage = useMemo(() => irDraftStorage(localForageStorageForScope(userId), `ir-draft:${userId}:${persistenceKey}`), [userId, persistenceKey]);
    const [open, setOpen] = useState(false);
    const [busy, setBusy] = useState(false);
    const [ready, setReady] = useState(false);
    const [saved, setSaved] = useState<IRPromptDraft | null>(null);
    const [error, setError] = useState("");
    const [restore, setRestore] = useState<{ original: string; applied: string } | null>(null);
    const controller = useRef<AbortController | null>(null);
    const lock = useRef(false);
    const hasRequested = Boolean(saved);
    const pending = saved?.status === "running" || saved?.status === "submitting";
    const stale = Boolean(saved && saved.fingerprint !== fingerprint);
    const draft = saved?.draft || "";
    const original = saved?.original || "";
    const input = saved?.input;

    async function watch(record: IRPromptDraft, abort: AbortController, initial?: IRPromptJob) {
        setBusy(true);
        try {
            let job = initial || await resumeIRPrompt(record, abort.signal);
            const deadline = Date.now() + 31 * 60_000;
            while (job.status === "running") {
                if (Date.now() > deadline) throw new Error("本次等待已结束，可继续查询原任务；请勿重复提交");
                await waitIRPoll(abort.signal);
                job = await queryIRPrompt(job.id, abort.signal);
            }
            if (abort.signal.aborted) return;
            const next: IRPromptDraft = { ...record, status: job.status, draft: job.prompt || "", error: job.error };
            await storage.write(next);
            if (!abort.signal.aborted) { setSaved(next); setError(job.status === "failed" ? job.error || "IR 优化失败" : ""); }
        } catch (failure) {
            if (!abort.signal.aborted) setError(failure instanceof Error ? failure.message : "IR 查询失败，可继续查询原任务");
        } finally { if (!abort.signal.aborted) { setBusy(false); lock.current = false; } }
    }

    useEffect(() => {
        const abort = new AbortController(); controller.current = abort;
        setReady(false); setSaved(null); setError(""); setBusy(false); setRestore(null); lock.current = false;
        void storage.read().then((record) => {
            if (abort.signal.aborted) return;
            setSaved(record); setReady(true);
            if (record && (record.status === "running" || record.status === "submitting")) {
                lock.current = true;
                void watch(record, abort);
            }
        }).catch(() => { if (!abort.signal.aborted) setError("无法读取 IR 任务记录，请恢复浏览器存储后刷新页面，以免重复提交"); });
        return () => { abort.abort(); controller.current?.abort(); };
        // The storage identity captures user + shot/node/conversation.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [storage]);

    async function start() {
        if (lock.current || pending || !ready || disabled || !prompt.trim()) return;
        lock.current = true;
        controller.current?.abort();
        const abort = new AbortController(); controller.current = abort;
        setOpen(true); setBusy(true); setError("");
        let submittedRecord: IRPromptDraft | undefined;
        try {
            const input = { ...await prepare(), requestId: crypto.randomUUID() };
            if (abort.signal.aborted) return;
            const record: IRPromptDraft = { id: input.requestId, original: prompt, fingerprint, input, status: "submitting", draft: "" };
            await storage.write(record);
            submittedRecord = record;
            setSaved(record);
            if (abort.signal.aborted) return;
            const job = await createIRPrompt(input);
            const accepted: IRPromptDraft = { ...record, status: job.status, draft: job.prompt || "", error: job.error };
            await storage.write(accepted);
            if (abort.signal.aborted) return;
            setSaved(accepted);
            await watch(accepted, abort, job);
        } catch (failure) {
            if (submittedRecord && failure instanceof ApiError && [400, 401, 403, 404, 422, 429].includes(failure.status || 0)) {
                const rejected: IRPromptDraft = { ...submittedRecord, status: "failed", error: failure.message };
                await storage.write(rejected).catch(() => {});
                if (!abort.signal.aborted) setSaved(rejected);
            }
            if (!abort.signal.aborted) setError(failure instanceof Error ? failure.message : "IR 优化失败");
        } finally { if (!abort.signal.aborted) { lock.current = false; setBusy(false); } }
    }
    return <>
        <Button size="small" disabled={disabled} onClick={() => setOpen(true)} title="打开提示词优化面板，不会自动发送请求">{busy ? "提示词优化 · 处理中" : "提示词优化"}</Button>
        {restore && prompt === restore.applied ? <Button size="small" type="text" disabled={disabled || busy} onClick={() => { onApply(restore.original); setRestore(null); }}>恢复优化前</Button> : null}
        <AppDrawer title="提示词优化" open={open} onClose={() => setOpen(false)} size={640}>
            <div data-canvas-no-zoom data-canvas-wheel-scroll className="flex flex-col gap-4">
                <details open={!hasRequested}><summary>{hasRequested ? "本次请求的原始需求" : "当前视频需求"}</summary><Typography.Paragraph style={{ whiteSpace: "pre-wrap" }}>{displayIRPrompt(hasRequested ? original : prompt, input?.assets) || "请先在视频提示词输入框中填写需求。"}</Typography.Paragraph></details>
                <Typography.Text type="secondary">发送时使用当前视频提示词、参考素材和生成设置。打开此面板不会发送请求。</Typography.Text>
                <Button type={draft ? "default" : "primary"} loading={busy} disabled={!ready || busy || pending || disabled || !prompt.trim()} onClick={() => void start()}>{hasRequested ? "重新优化提示词" : "开始优化提示词"}</Button>
                {pending && !busy ? <Button onClick={() => { if (!saved || lock.current) return; lock.current = true; setError(""); const abort = new AbortController(); controller.current = abort; void watch(saved, abort); }}>继续查询原任务（不重复生成）</Button> : null}
                {busy ? <Alert type="info" showIcon title="正在分析素材并编写提示词…" description="关闭面板、切换镜头或刷新后，可回到同一入口继续查询。请勿重复提交。" /> : null}
                {error ? <Alert type="error" showIcon title={error} description="原提示词未修改，你仍可使用原提示词生成视频。" /> : null}
                {stale ? <Alert type="warning" showIcon title="提示词、素材或设置已经变化" description="此结果基于优化前的输入，请重新优化以避免引用错位。" /> : null}
                <label htmlFor="ir-prompt-draft">优化后的提示词（可编辑）</label>
                <Input.TextArea id="ir-prompt-draft" value={draft} onChange={(event) => { if (!saved) return; const next = { ...saved, draft: event.target.value }; setSaved(next); void storage.write(next).catch(() => setError("优化后的编辑内容未能保存到浏览器，请复制备份后重试")); }} disabled={busy || pending || !hasRequested} placeholder="开始优化后，返回的提示词会显示在这里，你可以继续编辑。" autoSize={{ minRows: 16, maxRows: 28 }} />
                {input ? <Typography.Text type="secondary">参考素材：{input.assets.map((asset) => asset.label).join("、") || "无"}；{input.seconds} 秒 · {input.ratio} · {input.resolution}P</Typography.Text> : null}
                <Typography.Text type="secondary">本次优化不会改变素材和生成设置。应用后素材引用会绑定到原文件；删除或替换素材后请重新优化。请手动点击生成视频。</Typography.Text>
                <Space><Button onClick={() => setOpen(false)}>关闭</Button><Button type="primary" disabled={busy || disabled || stale || !draft.trim()} onClick={() => {
                    if (!saved || !input) return;
                    try {
                        const applied = bindIRPrompt(draft, input.assets, saved.fingerprint, fingerprint);
                        setRestore({ original: prompt, applied }); onApply(applied); setOpen(false);
                    } catch (failure) { setError(failure instanceof Error ? failure.message : "IR 素材引用无效"); }
                }}>应用到提示词</Button></Space>
            </div>
        </AppDrawer>
    </>;
}

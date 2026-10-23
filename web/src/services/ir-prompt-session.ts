import { createIRPrompt, queryIRPrompt, type IRPromptInput, type IRPromptJob } from "@/services/api/context-ir";
import { ApiError } from "@/services/api/request";
import type { StateStorage } from "zustand/middleware";

export type IRPromptDraft = {
    id: string; original: string; fingerprint: string; input: IRPromptInput;
    status: "submitting" | "running" | "completed" | "failed"; draft: string; error?: string;
};

const writes = new Map<string, Promise<unknown>>();
export function irDraftStorage(storage: StateStorage, key: string) {
    return {
        async read(): Promise<IRPromptDraft | null> {
            await writes.get(key);
            const raw = await storage.getItem(key);
            if (!raw) return null;
            const value = JSON.parse(raw) as IRPromptDraft;
            if (!value.id || value.input?.requestId !== value.id) throw new Error("IR 任务缓存无效，请联系管理员");
            return value;
        },
        write(value: IRPromptDraft) {
            const previous = writes.get(key) || Promise.resolve();
            const write = previous.catch(() => {}).then(async () => {
                const raw = await storage.getItem(key);
                const existing = raw ? JSON.parse(raw) as IRPromptDraft : null;
                if (existing?.id === value.id && (existing.status === "completed" || existing.status === "failed") && (value.status === "submitting" || value.status === "running")) return;
                await storage.setItem(key, JSON.stringify(value));
            });
            writes.set(key, write);
            void write.finally(() => { if (writes.get(key) === write) writes.delete(key); }).catch(() => {});
            return write;
        },
    };
}

// Resume only the same idempotent request, never create a new task after reload.
export async function resumeIRPrompt(saved: IRPromptDraft, signal: AbortSignal,
    dependencies = { query: queryIRPrompt, create: createIRPrompt }): Promise<IRPromptJob> {
    try { return await dependencies.query(saved.id, signal); }
    catch (error) {
        if (!signal.aborted && saved.status === "submitting" && error instanceof ApiError && error.status === 404) {
            return dependencies.create(saved.input, signal);
        }
        throw error;
    }
}

export function waitIRPoll(signal: AbortSignal) {
    return new Promise<void>((resolve, reject) => {
        const cancel = () => { clearTimeout(timer); reject(new DOMException("已停止等待", "AbortError")); };
        const timer = setTimeout(() => { signal.removeEventListener("abort", cancel); resolve(); }, 2000);
        if (signal.aborted) cancel();
        else signal.addEventListener("abort", cancel, { once: true });
    });
}

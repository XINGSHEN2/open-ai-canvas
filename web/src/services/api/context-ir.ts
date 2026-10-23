import { http } from "@/services/api/request";

export type IRAsset = { resourceId: string; kind: "image" | "video" | "audio"; label: string };
export type IRPromptInput = { requestId?: string; prompt: string; assets: IRAsset[]; seconds: number; ratio: string; resolution: string };
export type IRPromptJob = { id: string; status: "running" | "completed" | "failed"; prompt?: string; error?: string };
export function createIRPrompt(input: IRPromptInput, signal?: AbortSignal) {
    return http.post<IRPromptJob>("/context-ir/jobs", input, { signal });
}
export function queryIRPrompt(id: string, signal?: AbortSignal) {
    return http.get<IRPromptJob>(`/context-ir/jobs/${encodeURIComponent(id)}`, { signal });
}

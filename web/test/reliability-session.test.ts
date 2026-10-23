import { expect, test } from "bun:test";
import { completeRegistration } from "../src/lib/complete-registration";
import { loadModelCatalogWithRetry } from "../src/lib/model-catalog-load";
import { irDraftStorage, resumeIRPrompt, type IRPromptDraft } from "../src/services/ir-prompt-session";
import { ApiError } from "../src/services/api/request";

test("registration distinguishes POST failure from session initialization failure", async () => {
    let creates = 0;
    const result = await completeRegistration(async () => { creates++; }, async () => { throw new Error("network"); });
    expect(result.sessionReady).toBe(false);
    expect(creates).toBe(1);
    let initialized = false;
    await expect(completeRegistration(async () => { throw new Error("registration denied"); }, async () => { initialized = true; })).rejects.toThrow("registration denied");
    expect(initialized).toBe(false);
    expect((await completeRegistration(async () => {}, async () => {})).sessionReady).toBe(true);
});

test("catalog load retries boundedly and stops when account changes", async () => {
    let attempts = 0;
    const result = await loadModelCatalogWithRetry(async () => { if (++attempts < 3) throw new Error("offline"); return ["h3"]; }, () => true, async () => {});
    expect(result).toEqual(["h3"]);
    expect(attempts).toBe(3);
    attempts = 0;
    await expect(loadModelCatalogWithRetry(async () => { attempts++; throw new Error("offline"); }, () => true, async () => {})).rejects.toThrow("offline");
    expect(attempts).toBe(3);
    attempts = 0;
    let current = true;
    const cancelled = await loadModelCatalogWithRetry(async () => { attempts++; throw new Error("offline"); }, () => current, async () => { current = false; });
    expect(cancelled).toBeUndefined();
    expect(attempts).toBe(1);
});

const draft: IRPromptDraft = { id: "same-id", original: "original", fingerprint: "input", input: { requestId: "same-id", prompt: "test", assets: [], seconds: 5, ratio: "16:9", resolution: "480" }, status: "running", draft: "" };

test("IR survives remount with user/shot isolation and ordered draft saves", async () => {
    const data = new Map<string, string>();
    const storage = { getItem: (key: string) => data.get(key) || null, setItem: (key: string, value: string) => { data.set(key, value); }, removeItem: (key: string) => { data.delete(key); } };
    const first = irDraftStorage(storage, "user1:shot1");
    await first.write(draft);
    const restored = await irDraftStorage(storage, "user1:shot1").read();
    expect(restored?.id).toBe(draft.id);
    expect(await irDraftStorage(storage, "user2:shot1").read()).toBeNull();
    expect(await irDraftStorage(storage, "user1:shot2").read()).toBeNull();
    await Promise.all([first.write({ ...draft, status: "completed", draft: "edited result" }), first.write(draft)]);
    expect((await first.read())?.draft).toBe("edited result");
});

test("IR recovery queries existing task and never resubmits with a new ID", async () => {
    let creates = 0;
    const signal = new AbortController().signal;
    const job = await resumeIRPrompt(draft, signal, { query: async (id) => ({ id, status: "completed", prompt: "ready" }), create: async () => { creates++; throw new Error("unexpected create"); } });
    expect(job.prompt).toBe("ready");
    expect(creates).toBe(0);
    await resumeIRPrompt({ ...draft, status: "submitting" }, signal, {
        query: async () => { throw new ApiError("not found", { status: 404 }); },
        create: async (input) => { creates++; expect(input.requestId).toBe(draft.id); return { id: draft.id, status: "running" }; },
    });
    expect(creates).toBe(1);
});

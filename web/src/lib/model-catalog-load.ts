import { create } from "zustand";

export const useModelCatalogLoad = create<{ status: "idle" | "loading" | "ready" | "error"; userId: string }>(() => ({ status: "idle", userId: "" }));

export async function loadModelCatalogWithRetry<T>(load: () => Promise<T>, current: () => boolean,
    delay: (milliseconds: number) => Promise<void> = (ms) => new Promise((resolve) => setTimeout(resolve, ms))) {
    for (let attempt = 0; attempt < 3; attempt++) {
        if (!current()) return undefined;
        try { return await load(); }
        catch (error) {
            if (attempt === 2 || !current()) throw error;
            await delay(500 * (attempt + 1));
        }
    }
    return undefined;
}

import { describe, expect, test } from "bun:test";
import { QueryClient } from "@tanstack/react-query";
import { updateChapterListCache } from "../src/pages/projects/detail/chapter-list-cache";
import type { ProjectUnit } from "../src/services/api/projects";

const unit = (id: string, position: number) => ({ id, position, title: id, sourceText: "", kind: "chapter" } as ProjectUnit);
const key = ["project", "p", "units"];
function setup() {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
    client.setQueryData(key, { units: [unit("old", 0)], canvasCounts: { old: 2 } });
    return client;
}
function list(client: QueryClient) {
    return client.getQueryData<{ units: ProjectUnit[]; canvasCounts: Record<string, number> }>(key)!;
}

describe("chapter acknowledged writes and navigation cache", () => {
    test("new chapter is selectable before the background refresh completes", async () => {
        const client = setup();
        await updateChapterListCache(client, "p", [unit("new", 1)]);
        expect(list(client).units.map((u) => u.id)).toEqual(["old", "new"]);
        expect(list(client).canvasCounts.old).toBe(2);
    });
    test("late stale request cannot erase a successfully created chapter", async () => {
        const client = setup();
        let finish!: (value: unknown) => void;
        const fetch = client.fetchQuery({ queryKey: key, queryFn: () => new Promise((resolve) => { finish = resolve; }) }).catch(() => undefined);
        await updateChapterListCache(client, "p", [unit("new", 1)]);
        finish({ units: [unit("old", 0)], canvasCounts: {} });
        await fetch;
        await Promise.resolve();
        expect(list(client).units.map((u) => u.id)).toEqual(["old", "new"]);
    });
    test("batch import preserves all chapters, order and deduplicates IDs", async () => {
        const client = setup();
        await updateChapterListCache(client, "p", [unit("second", 2), unit("first", 1), unit("old", 0)]);
        expect(list(client).units.map((u) => u.id)).toEqual(["old", "first", "second"]);
    });
    test("deletion removes chapter and counts before fallback navigation", async () => {
        const client = setup();
        await updateChapterListCache(client, "p", [unit("next", 1)]);
        await updateChapterListCache(client, "p", [], ["old"]);
        expect(list(client).units.map((u) => u.id)).toEqual(["next"]);
        expect(list(client).canvasCounts).toEqual({});
    });
    test("deleting last chapter produces a valid empty list", async () => {
        const client = setup();
        await updateChapterListCache(client, "p", [], ["old"]);
        expect(list(client).units).toEqual([]);
    });
    test("first chapter in an empty project is immediately available", async () => {
        const client = new QueryClient();
        await updateChapterListCache(client, "p", [unit("first", 0)]);
        expect(list(client).units[0].id).toBe("first");
    });
    test("only the specified project cache is changed", async () => {
        const client = setup();
        await updateChapterListCache(client, "other", [unit("other", 0)]);
        expect(list(client).units.map((u) => u.id)).toEqual(["old"]);
    });
});

import type { QueryClient } from "@tanstack/react-query";
import type { ProjectUnit } from "@/services/api/projects";

type ChapterList = { units: ProjectUnit[]; canvasCounts: Record<string, number> };

/** Commit acknowledged writes before navigating; an older in-flight list must not undo them. */
export async function updateChapterListCache(client: QueryClient, projectId: string, incoming: ProjectUnit[], removedIds: string[] = []) {
    const key = ["project", projectId, "units"];
    await client.cancelQueries({ queryKey: key, exact: true });
    const removed = new Set(removedIds);
    client.setQueryData<ChapterList>(key, (current) => {
        const byId = new Map((current?.units || []).map((unit) => [unit.id, unit]));
        for (const unit of incoming) byId.set(unit.id, unit);
        for (const id of removed) byId.delete(id);
        return {
            ...current,
            units: [...byId.values()].sort((left, right) => left.position - right.position),
            canvasCounts: Object.fromEntries(Object.entries(current?.canvasCounts || {}).filter(([id]) => !removed.has(id))),
        };
    });
}

import { useEffect, useRef } from "react";

// Polling replaces detail objects frequently. Only hydrate changed server content,
// and never replace an unsaved draft belonging to the same shot.
export function useShotDraftHydration(shotId: string, snapshot: string, dirty: boolean, hydrate: () => void) {
    const hydrated = useRef<{ shotId: string; snapshot: string } | null>(null);
    useEffect(() => {
        const previous = hydrated.current;
        if (previous?.shotId === shotId && (dirty || previous.snapshot === snapshot)) return;
        hydrated.current = { shotId, snapshot };
        hydrate();
    });
}

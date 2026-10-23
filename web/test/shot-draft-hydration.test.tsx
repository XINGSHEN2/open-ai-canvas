import { expect, test } from "bun:test";
import { Window } from "happy-dom";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { useShotDraftHydration } from "../src/pages/projects/detail/use-shot-draft-hydration";

test("polling and binding refresh preserve a dirty shot; clean revisions and shot switches hydrate", async () => {
    const browser = new Window();
    const previousWindow = globalThis.window;
    const previousDocument = globalThis.document;
    Object.assign(globalThis, { window: browser, document: browser.document, IS_REACT_ACT_ENVIRONMENT: true });
    const root = createRoot(document.createElement("div"));
    let text = "";
    let writes = 0;
    function Harness(props: { shot: string; snapshot: string; dirty: boolean }) {
        useShotDraftHydration(props.shot, props.snapshot, props.dirty, () => { text = props.snapshot; writes++; });
        return null;
    }
    const render = async (shot: string, snapshot: string, dirty: boolean) => {
        await act(async () => root.render(createElement(Harness, { shot, snapshot, dirty })));
    };
    try {
        await render("A", "revision-1", false);
        text = "UNSAVED EDIT";
        await render("A", "revision-1", true);
        await render("A", "revision-1-new-binding", true);
        await render("A", "external-revision-2", true);
        expect(text).toBe("UNSAVED EDIT");
        expect(writes).toBe(1);
        await render("A", "saved-revision-3", false);
        expect(text).toBe("saved-revision-3");
        await render("A", "saved-revision-3", false);
        expect(writes).toBe(2);
        await render("B", "B-revision", true);
        expect(text).toBe("B-revision");
    } finally {
        await act(async () => root.unmount());
        Object.assign(globalThis, { window: previousWindow, document: previousDocument });
        await browser.happyDOM.close();
    }
});

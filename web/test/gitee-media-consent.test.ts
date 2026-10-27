import { afterEach, expect, test } from "bun:test";
import { registerGiteeMediaConsent, requestGiteeMediaConsent } from "../src/services/gitee-media-consent";

let cleanup: (() => void) | undefined;
afterEach(() => { cleanup?.(); cleanup = undefined; });
test("missing consent UI fails closed", async () => {
    await expect(requestGiteeMediaConsent()).rejects.toThrow("已取消");
});
test("cancel does not grant upload consent", async () => {
    cleanup = registerGiteeMediaConsent(async () => false);
    await expect(requestGiteeMediaConsent()).rejects.toThrow("已取消");
});
test("each submission asks again", async () => {
    let count = 0;
    cleanup = registerGiteeMediaConsent(async () => { count++; return true; });
    expect(await requestGiteeMediaConsent()).toBe(true);
    expect(await requestGiteeMediaConsent()).toBe(true);
    expect(count).toBe(2);
});

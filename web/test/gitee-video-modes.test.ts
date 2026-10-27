import { expect, test } from "bun:test";
import { bindGiteeFrames } from "../src/lib/gitee-video-modes";
test("selected frame model labels displayed input order", () => {
    expect(bindGiteeFrames([{id:"a"},{id:"b"}]).map(x=>x.role)).toEqual(["first_frame","last_frame"]);
});
test("explicit frame selection overrides array order", () => {
    expect(bindGiteeFrames([{id:"a"},{id:"b"}],{videoStartFrameNodeId:"b",videoEndFrameNodeId:"a"}).map(x=>x.role)).toEqual(["last_frame","first_frame"]);
});
test("keeps explicit roles", () => {
    expect(bindGiteeFrames([{id:"a",role:"last_frame"},{id:"b",role:"first_frame"}])[0].role).toBe("last_frame");
});
test("reject incomplete and duplicate frame selections", () => {
    expect(()=>bindGiteeFrames([{id:"a"}])).toThrow();
    expect(()=>bindGiteeFrames([{id:"a"},{id:"b"}],{videoStartFrameNodeId:"missing",videoEndFrameNodeId:"a"})).toThrow();
    expect(()=>bindGiteeFrames([{id:"a",role:"first_frame"},{id:"b",role:"first_frame"}])).toThrow();
});

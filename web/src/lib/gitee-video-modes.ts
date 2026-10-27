type FrameReference = { id: string; role?: string };
export function bindGiteeFrames<T extends FrameReference>(images: T[], metadata?: Record<string, unknown>): T[] {
    if (images.length !== 2) throw new Error("首尾帧生成需要两张图片：第一张为首帧，第二张为尾帧");
    const start = String(metadata?.videoStartFrameNodeId || "");
    const end = String(metadata?.videoEndFrameNodeId || "");
    const explicit = Boolean(start || end || images.some((image) => image.role === "first_frame" || image.role === "last_frame"));
    const resolved = images.map((image, index) => ({ ...image, role: explicit
        ? image.role === "first_frame" || image.role === "last_frame" ? image.role : image.id === start ? "first_frame" : image.id === end ? "last_frame" : "reference_image"
        : index === 0 ? "first_frame" : "last_frame" }));
    if (resolved.filter((image) => image.role === "first_frame").length !== 1 || resolved.filter((image) => image.role === "last_frame").length !== 1) {
        throw new Error("请明确指定一张首帧和一张尾帧，不能重复或遗漏");
    }
    return resolved;
}

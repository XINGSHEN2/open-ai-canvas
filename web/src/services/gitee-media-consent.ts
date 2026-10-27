export const GITEE_MEDIA_NOTICE = "参考图片、视频及音频将上传至第三方 Litterbox 公网临时托管，设置保留 12 小时，供 Gitee 下载。持有链接的人可能访问素材，请勿上传敏感或未获授权的内容。原素材和生成结果仍保存在本站。Gitee 调用会产生费用。";

let prompt: (() => Promise<boolean>) | undefined;
export function registerGiteeMediaConsent(handler: () => Promise<boolean>) {
    prompt = handler;
    return () => { if (prompt === handler) prompt = undefined; };
}
export async function requestGiteeMediaConsent() {
    if (!prompt || !(await prompt())) throw new Error("已取消 Gitee 生成，未上传到公网中转服务");
    return true;
}

import { useEffect } from "react";
import { App } from "antd";
import { GITEE_MEDIA_NOTICE, registerGiteeMediaConsent } from "@/services/gitee-media-consent";
import { useUserStore } from "@/stores/use-user-store";

export function GiteeMediaConsent() {
    const { modal } = App.useApp();
    const userID = useUserStore((state) => state.user?.id);
    useEffect(() => {
        if (!userID) return;
        const pending = new Set<() => void>();
        const unregister = registerGiteeMediaConsent(() => new Promise<boolean>((resolve) => {
            const dismiss = () => { dialog.destroy(); resolve(false); pending.delete(dismiss); };
            const dialog = modal.confirm({
                title: "使用 MiniMax-H3（Gitee）",
                content: GITEE_MEDIA_NOTICE,
                okText: "同意上传并生成", cancelText: "取消",
                onOk: () => { pending.delete(dismiss); resolve(true); },
                onCancel: dismiss,
            });
            pending.add(dismiss);
        }));
        return () => { unregister(); for (const dismiss of pending) dismiss(); };
    }, [modal, userID]);
    return null;
}

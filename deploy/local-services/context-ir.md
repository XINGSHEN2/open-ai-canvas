# 视频提示词 IR 优化

创作首页、画布视频节点、短剧镜头的视频提示词旁提供「IR 优化」。仅对 MiniMax H3 显示。
优化结果在侧栏展示，允许编辑，再点击「应用到提示词」。不会自动提交视频，不修改模型、时长、画幅或分辨率。应用后可恢复优化前原文；输入或素材变化后旧结果禁止应用，需重新优化。

## 部署

网站后端环境变量 `CANVAS_CONTEXT_IR_BASE_URL` 指向 Context-IR HTTP 服务；不配置时明确返回 503。
将其主机加入现有 `CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS` 精确放行列表。
`CANVAS_PUBLIC_BASE_URL` 必须能被 IR 和感知服务访问，用于既有资源签名链接，不要求公网暴露。
IR 保留自身的模型供应商与密钥配置，网站不新增供应商 Key 管理。

## 接口

- `POST /api/context-ir/jobs`：需登录，JSON 字段 `prompt`、`assets`、`seconds`、`ratio`、`resolution`。
- `assets` 为有序数组，每项 `resourceId`、`kind`（image/video/audio）、`label`。不接受任意 URL、本地路径、上游地址或感知服务配置。后端校验素材所属用户。
- `seconds` 为 5–15 的整数；`ratio` 为 16:9 / 9:16；`resolution` 为字符串 480 / 768。
- `GET /api/context-ir/jobs/:id`：同用户查询，返回 `id/status/prompt/error/createdAt/finishedAt`；使用网站统一 `{code,data,msg}` 信封。
- 状态为 running/completed/failed。最多同时两个请求，满额返回 429；没有无限排队。

网站后端异步调用 IR 的 `/api/h3/prompt`，只取 `result.h3_prompt`。不使用 IR 返回的 `h3_request`，避免其中默认的 768 覆盖用户选择的 480P。网络等待上限 30 分钟；不重试已发送的请求，避免重复计费。

## 当前边界

- 这是单实例适配器。结果以权限 0600 保存于后端数据目录的 context-ir-jobs 子目录，按用户隔离；进程重启后未完成任务标记中断，不自动重放。多副本部署前应迁移到公共任务队列。
- 关闭侧栏不取消 IR 工作；留在当前页面可再次查看结果。离开页面后后端任务仍可能继续，但当前版本没有历史任务选择器。
- 暂不自动复用素材分析，以免不同用户需求复用不完整证据。
- 参考素材必须已经上传为网站资源。未同步素材会明确提示，不忽略它们。感知服务必须能读取短时签名链接。
- 任务结果文件暂不自动清理；管理员应按数据保留策略清理终态结果。
- 必须在部署后验收实际登录、三处入口、图片/视频/音频读取、编辑应用、恢复原文及最终 H3 输入。本地单元测试和构建不代表实际上游生成已验证。

# 本地模型接入配置（未应用）

`local-models.json` 是管理员渠道与模型请求模板，当前没有写入数据库。已有管理员账号和定价保持不变。`apiKey=local-no-auth` 是无鉴权内网服务使用的公开占位符，不是秘密或实际授权凭据。

## 渠道与协议

| 模型 | 渠道地址 | 协议 |
| --- | --- | --- |
| Qwen Image 2.1 | `http://10.42.1.1:9008/v1` | `openai-image`，同步 Images + multipart |
| MiniMax H3 Ref2VA | `http://10.42.1.2:30011` | `minimax-video`，复用已有 V2 创建/轮询 |

H3 已有对应兼容接口，不新增一套重复队列或更名成 OpenAI Videos。创建使用 `/v2/video_generation`，查询 `/v2/query/video_generation/{id}`；必须选内置 `minimax-hailuo-video-v2` 声明式插件，不能使用老式通用视频协议。上游 `providerModelKey` 保持 `MiniMax-H3`，网站模型 key 使用 `minimax-h3-ref2va`。

## 能力限制

- H3：仅 `reference_to_video`；480P/768P；5–15 秒整数；比例 21:9、16:9、4:3、1:1、3:4、9:16；图片≤9、视频≤3、音频≤3；视频和音频参考先采用 15 秒保守上限。不提供首尾帧/FL2VA、无参考文生视频、水印或独立音频开关。H3 本身输出音视频，音频不是可切换生成能力。
- Qwen：n=1，最多10张参考图；像素维度32对齐；预置1K/2K尺寸与比例映射，默认2048×2048。网站仅展示服务可接受的保守预设子集，并非宣称模型只能这些尺寸；实际服务仍接受其它32对齐尺寸。1K/2K是网站预设档位名称，不代表所有比例的短边固定为1024/2048。
- Qwen 禁用蒙版、质量、透明背景请求开关和多输出。输出仍可保留模型实际产生的 RGBA，但不能请求模型未实现的 `background` 参数。
- 网站应使用现有 capabilityConfig 驱动按钮和校验，不全局禁用其它云模型的功能。

## 应用时的步骤

1. 网站后端启动环境设置 `CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS=10.42.1.1,10.42.1.2`，不能允许全部私网。保留现有其它精确白名单主机。
2. `CANVAS_PUBLIC_BASE_URL=http://10.42.1.2:3000`；参考素材使用已有素材下载/签名链路，不能传浏览器 blob URL 或容器本地路径给 H3。
3. 管理员创建模板中的两个 channel，再分别保存 model 的 capabilityConfig。模板故意使用 `priceConfigured=false`；由管理员明确设置内部免费或价格档，不自动变更收费策略。
4. 保存模型会通过项目已有服务校验并投影到前台模型目录。未完成定价/启用配置之前，不能声称模型已在下拉框可用。
5. 验证图片单图、参考图编辑、H3混合参考视频生成；检查参数按钮、实际提交尺寸/时长、错误和结果持久化。

## 验证命令

在 backend 中运行：

```bash
go run ./cmd/check-local-model-profiles ../deploy/local-services/local-models.json
```

只验证能力模板，不读账号密码，不写数据库，不接纳推理任务。H3通用插件的默认4–30秒/2K不能覆盖本模板的5–15秒/480P/768P限制。最终浏览器端到端验证需要管理员应用配置后执行。

当前仅用于受信内网试运行；正式使用真实密钥应启用 HTTPS。

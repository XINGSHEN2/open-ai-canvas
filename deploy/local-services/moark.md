# GLM-5.3（Moark）

直接使用网站现有的管理员后台 → 渠道管理。无需环境文件、导入脚本或专用配置命令。

已创建的渠道名为 `Moark GLM-5.3`，等待管理员填写真实 API Key 并启用渠道和模型：

- 接口地址：`https://api.moark.com/v1`
- 模型 ID：`GLM-5.3`
- 协议：OpenAI Chat Completions（`chat-completion`）
- 附加请求头：`X-Failover-Enabled: true`

API Key 通过网站原有机制加密保存到数据库，不提交 Git。
网站已有文本流式协议和推理内容解析器；尚未验证真实 Key 对应的模型权限及上游回复。
只开放文本输入，不声明图片理解或 Agent 能力。

访客额外允许这个准确上游、模型和协议组合。渠道当前按内部零积分配置，
实际 API 费用由 Moark Key 所属账号承担。

网站当前保守配置为上下文预算 8192 token、输出上限 1024 token、提示词
12000 字符；这些不是模型官方最大能力声明。示例中的 temperature、top_p、
top_k、frequency_penalty 未固定为网站默认值。

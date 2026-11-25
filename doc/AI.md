# AI 辅助审核

[返回 README](../README.md) · [配置参考](CONFIGURATION.md)

AI 是可选功能，默认关闭。开启后，投稿文字、话题和附图数量会发送到所选服务，提供低质量标记和简短原因。默认不发送图片；开启「审核附图」后，会额外发送受大小和数量限制的附图。不额外附带用户账号，但投稿本身可能包含个人信息。结果供后台复核，不自动删除投稿；失败或超时不会丢弃提问。

## 在后台配置

使用站点所有者账号进入「系统设置 → AI 辅助审核」。

1. 选择接口协议，填写服务提供方的 **Base URL** 和 **API Key**。
2. 点击「获取模型」，在模型输入框中选择或搜索，也可以直接填写自定义模型 ID。部分兼容服务不提供模型列表，此时手动填写即可。
3. 填写低质量判定标准；思考和兼容性选项可先保持默认。
4. 点击「测试连接与模型」。可填写测试文字；启用图片审核时会附带程序生成的示例图片。测试会消耗上游用量，不读取站内真实投稿。
5. 勾选开启审核并「保存 AI 设置」。立即用于新投稿，无需重启；旧投稿可在后台选择补审，任务记录保存在数据库中，重启后仍保留。升级不会自动发起批量审核。

测试和获取模型使用当前表单内容，**不会自动保存**。获取模型不会覆盖已填写的模型 ID，也不保证列表中的每个模型都能处理文字或图片审核；用连接测试确认所选模型可用。

## 支持的协议

| 协议 | 基础地址示例 | 请求路径 | 思考等级字段 |
| --- | --- | --- | --- |
| OpenAI Chat Completions | `https://api.openai.com/v1` | `/chat/completions` | `reasoning_effort` |
| OpenAI Responses | `https://api.openai.com/v1` | `/responses` | `reasoning.effort` |
| Anthropic Messages | `https://api.anthropic.com/v1` | `/messages` | `output_config.effort` |

基础地址不包含右侧的请求路径。只填写域名时会自动追加 `/v1`；带有自定义路径的网关地址会原样保留。模型列表通过基础地址下的 `/models` 获取，Anthropic 支持分页。

选择对应协议后，可使用兼容网关或其他厂商的地址与模型。远程服务要求 HTTPS，本机和内网 IP 支持 HTTP。请求不会跟随重定向，以免把密钥发送到其他地址。OpenAI 使用 Bearer 鉴权；Anthropic 使用 `x-api-key` 与 `anthropic-version`。

DeepSeek 等 Chat Completions 兼容服务如果不接受 `max_completion_tokens`，在兼容性选项中改为 `max_tokens`。升级前已配置的 DeepSeek 服务会保留原有模型和审核规则，并将旧密钥迁移为加密存储。

## 思考与输出预算

默认不发送思考参数，交给服务决定。可选等级不代表每个模型都支持：OpenAI 协议可指定 `none`、`minimal`、`low`、`medium`、`high`、`xhigh`、`max`；Anthropic 协议可指定 `low` 到 `max`。请按具体模型选择，不支持的组合会在连接测试中报错。参阅 [OpenAI 推理指南](https://developers.openai.com/api/docs/guides/reasoning)和 [Anthropic effort 文档](https://platform.claude.com/docs/en/build-with-claude/effort)。

Anthropic 还可指定思考模式：服务默认、自适应、手动预算、关闭或工具间思考。手动预算至少为 1024 Token，且小于输出 Token 上限；其他模式是否适用取决于模型。普通提问审核无需工具间思考，保持默认即可。

输出 Token 上限默认 4096，推理模型可能把其中一部分用于思考。若测试提示输出被截断，可提高预算或降低思考等级。单次审核最长约两分钟；不会阻塞投稿保存。

## 密钥与备份

只有站点所有者可以查看 AI 配置、测试和修改服务；接口仅返回“是否已配置密钥”，不会回传密钥本身。修改其他字段时留空保留密钥，更换服务地址或协议时需重新填写对应密钥。勾选清除密钥时同时关闭审核。

密钥加密保存在数据库中，依赖实例的 `security.bilibili_cookie_key`。备份、迁移和升级时，必须保留原有密钥（`data/instance.json` 或自行配置的密钥），否则已保存的 AI 密钥无法解密。这些文件不属于源码仓库。上游原始响应、密钥和投稿内容不会写入 AI 错误日志。

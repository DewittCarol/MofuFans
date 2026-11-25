# MofuFans 运行包

此目录包含 Go 可执行文件、前后台静态页面和部署模板。构建时没有加入实际配置或业务数据。

运行端需要兼容的 Linux / CPU / C 运行库，以及 Nginx 和 CA 证书，无需安装 Go 或 Node.js。

首次安装到 `/opt/mofufans` 后，按照[部署指南](../doc/DEPLOYMENT.md#3-初始化安装目录)设置用户和权限，安装 `deploy/mofufans.service` 与 `deploy/nginx-native.conf`。启动后从服务日志复制一次性安装码，打开 `/admin/` 完成网页引导。已经把运行包放入安装目录时，跳过指南中的复制构建产物步骤。

默认访问地址为 `http://127.0.0.1:8088/`，后台位于 `/admin/`。默认 SQLite 和本地存储即可使用，外部服务可以后续在后台配置：

- 「系统设置 → 部署与集成」：本地／私有阿里云 OSS、连接检查、私信测试 UID 和 HTTPS Cookie。
- 「系统设置 → AI 辅助审核」：OpenAI Chat Completions／Responses 或 Anthropic Messages，支持自定义服务、模型获取、思考参数和连接测试；默认关闭。配置步骤见 [AI 指南](../doc/AI.md)。
- 「主题管理」：品牌、配色、导航、背景、鼠标、表情合集和页脚 ICP／公安备案信息。

配置优先级、OSS 文件迁移和数据备份见 [配置参考](../doc/CONFIGURATION.md)。数据库、实例密钥和运行配置均需持久保存，不属于可重新生成的程序文件。

升级时保留已有 `config/config.json` 和 `data/`，只替换程序与 `web/`，并参考新版配置模板合并新增项。

MofuFans 基于 [Xinrea/JoiAsk](https://github.com/Xinrea/JoiAsk) 继续开发，使用 [MIT License](../LICENSE)。原作者与素材说明见 [NOTICE.md](../NOTICE.md)。

# 配置参考

[返回 README](../README.md) · [部署指南](DEPLOYMENT.md)


## 默认启动与网页配置

没有配置文件也能启动：默认使用 SQLite 和本地文件，自动生成两把独立随机密钥。打开 `/admin/`，用启动日志或 `data/setup-token` 中的一次性安装码完成引导。未初始化时，除引导和基础主题等必要接口外，业务 API 暂不开放；创建管理员后安装入口关闭，安装码文件删除。已有管理员的实例会直接显示登录页。

后台「系统设置 → 部署与集成」支持配置本地／OSS 存储、私信测试 UID、HTTPS Cookie。投稿权限、B 站登录、已安装鼠标套装选择、桌宠默认展开状态和 [AI 辅助审核](AI.md)位于系统设置的相应分区。品牌、配色、背景、表情素材、页脚备案，以及桌宠地址／名称／显示开关位于「主题管理」。OSS 和主播扫码会话可稍后配置；粉丝签名验证读取公开签名，不需要开放平台应用或验证账号 Cookie。

品牌／模块图片、背景和表情有上传入口；**桌宠 VRM 没有上传入口或动作编辑器，自定义鼠标没有套装创建或上传入口**。已有兼容模型可自行托管后填写地址，使用前须确认许可；鼠标默认仅含系统指针，新增套装需准备精灵图与 manifest，登记嵌入 Go 的清单并重新构建部署。专属动作和特殊模型需适配，具体边界见[功能与适配说明](FEATURES.md)。

| 文件 | 用途 |
| --- | --- |
| `data/instance.json` | 自动生成的会话、加密密钥，重启后复用 |
| `data/settings.json` | 后台保存的存储与集成设置，包含所配置的 OSS 凭证 |
| `data/setup-token` | 仅初始化前存在的安装码 |
| `config/config.json` | 可选的部署预配置 |

这些文件仅在服务器保存，不应提交仓库或公开下载；请把整个 `data/` 与部署配置一起备份。Linux 下自动创建的密钥和配置文件仅文件所有者可读写。丢失加密密钥后，已有 B 站登录、AI 密钥、礼物凭据和地址等加密数据不能用新密钥恢复。

## 可选的部署预配置

结构见 [config/config.example.json](../config/config.example.json)。默认读取可选的 `config/config.json`，可用 `MOFUFANS_CONFIG` 明确指定其他文件；明确指定的文件不存在会报错。

部署与集成项的优先级为：**环境变量 > 后台保存值 > 配置文件 > 内置默认值**。环境变量控制的字段在后台标记并禁用编辑。配置文件提供初始值，之后可以在后台调整；如需强制覆盖已保存值，请使用环境变量。数据库、监听端口、目录以及会话与加密密钥仅通过部署配置设置，不在后台更换。

仅在全新实例中，可选运行 `python3 scripts/init-config.py` 生成独立密钥与配置。已有配置或实例数据时会拒绝生成，升级必须保持原密钥不变。管理员在网页引导中创建。高级自动化可提供 `bootstrap_admin`，其密码必须是前端 MD5 传输值的 bcrypt 哈希，不能填明文或单独的 MD5；修改它不会重置已有账号。自行编写配置时也可以只填写需要覆盖的字段，省略的内容使用默认值。

配置可用 `MOFUFANS_` 环境变量覆盖，层级的点改为下划线，例如 `MOFUFANS_OSS_ACCESS_KEY`、`MOFUFANS_OSS_SECRET_KEY`。Compose 用户须在自己维护的私有覆盖文件中把变量传入容器，容器不会自动继承宿主机所有变量。

| 配置 | 含义 |
| --- | --- |
| `security.session_secret` | 可选，至少 32 字符的随机会话密钥；未提供时自动生成并持久保存 |
| `security.bilibili_cookie_key` | 可选，独立随机加密密钥；未提供时自动生成，用于 B 站登录、AI 密钥、私信、礼物凭据和地址；已有数据库升级必须保持原值 |
| `security.secure_cookie` | HTTPS 部署为 `true`，本机 HTTP 调试为 `false` |
| `bootstrap_admin` | 可选的空库管理员预配置；密码为 bcrypt(MD5(password))，通常留空并使用网页引导 |
| `storage_type` | `local` 或 `oss` |
| `state_directory` | 自动生成密钥、安装码及后台配置的保存目录，默认 `./data`；修改它不会迁移数据库或上传文件 |
| `server.host` / `server.port` | API 监听地址，默认 `127.0.0.1:8080`；更改端口时同步修改 Nginx 的上游地址 |
| `server.allowed_origins` | 精确跨域来源列表；同域留空，不接受 `*`；浏览器写操作与实时连接也校验同源或显式允许来源 |
| `server.trusted_proxies` | 可信代理地址，默认仅信任回环代理，拒绝 `0.0.0.0/0`、`::/0` 等全地址 CIDR；应用按IP限速和封禁依赖此项按实际代理链路正确配置 |
| `bilibili.test_recipient_uid` | 自己控制的测试 UID，使用字符串，必须不同于发送账号；留空时不能准备私信队列 |
| `security.guard.policies` | 可选的内置访问预算覆盖，未填写时默认防护立即生效；有效字段和范围见[安全说明](SECURITY.md#默认资源预算) |

应用防护状态和封禁存于 `data/security/`（随 `state_directory` 变化），必须持久化。后台「安全」仅供站点所有者查看和管理，预算不在后台关闭；默认值足够时无需配置。误封恢复见[本地命令](SECURITY.md#安全记录误封与本地恢复)。

## 数据库

默认 SQLite：`./data/mofufans.db`，首次启动自动建表。数据库、文件和密钥须一起备份；先停服或使用 SQLite backup API，不能仅复制运行中的主数据库文件，详见[备份](DEPLOYMENT.md#备份)。更换数据库需要另行迁移数据，不会因修改连接信息自动迁移。

使用 MySQL 时先创建空数据库，再把下列片段合并到实际配置；密码使用私有配置或 `MOFUFANS_MYSQL_PASS` 提供：

```json
{
  "db_type": "mysql",
  "mysql": {
    "host": "your-mysql-host",
    "port": 3306,
    "user": "mofufans",
    "pass": "",
    "name": "mofufans"
  }
}
```

## 阿里云 OSS

创建自己的 **私有 Bucket**，使用只允许操作该 Bucket／项目前缀的 RAM 凭据。在首次引导或后台「部署与集成」选择 OSS，填写 Endpoint、地域、Bucket、前缀及凭证，再点击「检查 OSS 连接」。检查会执行临时对象的写入、列举、读取和删除；保存时也会检查。密钥只提交到本站服务端，不在查询接口中回显，留空表示保留已有凭证。

首次引导中的存储设置立即生效。已运行站点修改存储时，需要确认文件迁移安排，保存后重启 API 才生效；重启前继续使用原存储。测试 UID 和 Cookie 设置保存后立即生效。

也可以把下列片段合并到部署配置；如果已在后台保存过设置，以后台值为准，强制覆盖时使用环境变量：

```json
{
  "storage_type": "oss",
  "oss": {
    "endpoint": "https://oss-cn-hangzhou.aliyuncs.com",
    "region": "cn-hangzhou",
    "bucket": "your-private-bucket",
    "prefix": "mofufans",
    "access_key": "",
    "secret_key": ""
  }
}
```

Endpoint、Region 须对应 Bucket 所在地域，Endpoint 使用不带路径的 HTTPS `*.aliyuncs.com` 地址。密钥保存在服务端私有配置或环境变量，前台和下载请求不接触 AccessKey，不需要 Bucket 匿名读取或浏览器直传的 CORS 配置。

使用 OSS **V4 签名**，对上传对象显式设置 private ACL；参考 [阿里云签名升级说明](https://www.alibabacloud.com/help/en/oss/developer-reference/guidelines-for-upgrading-v1-signatures-to-v4-signatures)。

| 内容 | 默认 OSS 前缀 | 默认本地目录 |
| --- | --- | --- |
| 投稿图片、缓存头像 | `mofufans/uploads/` | `data/uploads/` |
| 背景 | `mofufans/backgrounds/` | `data/backgrounds/` |
| 品牌图标 | `mofufans/branding/` | `data/branding/` |
| 自定义表情 | `mofufans/emojis/` | `data/emojis/` |
| 画廊图片 | `mofufans/gallery/` | `data/gallery/` |
| 舰礼图片 | `mofufans/voyage/` | `data/voyage/` |
| 电子礼物附件 | `mofufans/digital-gifts/` | `data/digital-gifts/` |

RAM 权限应覆盖所选前缀内的读、写、删除、列举、对象 ACL 和分片上传／终止操作，Bucket 级列举也需允许应用使用的前缀。权限不足时请求失败，不会静默退回本地。可配置清理过期未完成分片的生命周期规则。

访问仍使用本站 URL。画廊审核、舰礼草稿、电子礼物密码及资格检查完成后，应用才流式读取私有对象；支持 Range 下载。**不要用公开 CDN 或 Nginx alias 暴露私密模块。**

OSS 模式仍须持久化 `data/`：实例密钥、后台保存的配置（包含 OSS 凭据）、SQLite（如使用）和分片上传临时文件均保留在服务器。需预留在传文件的空间；完成并记录成功后会清理临时文件。未完成上传 24 小时后过期，在后续上传时清理。这不是浏览器直传方案。

切换本地／OSS 不会自动复制旧文件。已有安装须停写、备份，按上表保留文件名复制并核对内容，再切换配置；未完成分片不要当成已完成礼物迁移。不要直接换 Bucket／prefix 后删除旧目录。

## 表情素材与合集

在「主题管理 → 表情素材」分别管理投稿表情与互动表情。投稿表情可按合集组织；互动表情用于卡片上的贴表情效果。默认包含 JoiAsk 原版轴伊动态表情与动画贴表情，文件随程序分发，不需要先配置 OSS。

可以新建和改名合集，上传 PNG、JPG、GIF 或 WebP（每张不超过 5 MB，每批最多 20 张），也可以添加原生 Emoji 作为互动表情。拖动手柄调整顺序，或用空格与方向键操作；更改自动保存。

删除单个表情后可通过当前页面的「撤销删除」恢复。删除合集会同时移除其中的可选表情，需确认且无法在后台撤销；内置合集、空合集和最后一个合集也可删除，之后可新建合集。已经删除的合集不会在服务重启时重新出现。

删除操作保留历史投稿需要的标记、图片及已有互动计数。它不会清理磁盘或 OSS 中的素材文件；如仍需正常显示历史内容，不要手动删除对应文件。

## 页脚备案信息

在「主题管理 → 页脚备案」填写 ICP 备案号和／或公安备案号，保存后显示在前台所有页面底部。留空时隐藏对应项目，不需要编辑前端代码。

ICP 备案号链接到工信部查询网站。公安备案支持粘贴官方查询链接；未填写链接时，可根据备案号中的 14 位数字自动生成。备案图标可上传，或填写 HTTPS 图片地址。请填写本站已经取得的信息，不提供示例备案号作为默认值。

这些信息保存于数据库，与主题预设独立；切换、导入或重置配色不会修改备案信息。

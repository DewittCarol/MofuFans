# 部署指南

[返回 README](../README.md) · [配置参考](CONFIGURATION.md) · [开发指南](DEVELOPMENT.md)

MofuFans 由一个 Go API 和两套静态页面组成。Nginx 提供前台、`/admin/` 后台，并把 `/api/` 和 `/upload-img/` 转发给 API。默认数据库为 SQLite，上传文件保存在 `data/`。

开放公网前请完成[上线与日常安全指南](SECURITY-OPERATIONS.md)的检查。让AI协助时也要提供这份指南，并要求它交付实际验收结果；“容器已启动”不代表HTTPS、权限和备份均已配置。

| 方式 | 构建环境 | 运行环境 |
| --- | --- | --- |
| Docker Compose | Docker 在镜像内完成编译 | Docker + Compose |
| Linux 源码部署 | Go 1.26.11+、Node.js 22.14+／npm、C 编译器 | Linux、Nginx、systemd |
| 构建包部署 | 在兼容的 Linux 机器上完成上述构建 | Linux、Nginx、systemd；无需 Go / Node.js |

首次配置在网页完成，不需要预先填写密钥或管理员密码。便捷启动和构建脚本需要 Python 3.9+；直接使用 Compose 或运行已构建的程序不需要 Python。Windows 可使用 Docker Desktop，需要原生 Linux 构建时使用 WSL。以下源码命令均在项目根目录执行。

## Docker Compose

安装 [Docker Engine / Desktop](https://docs.docker.com/get-started/get-docker/) 与 [Compose 插件](https://docs.docker.com/compose/install/)，确认 `docker compose version` 和 `docker info` 能正常运行。

一条命令检查环境、构建镜像并启动：

```bash
python3 scripts/deploy.py
```

Windows 将 `python3` 换成 `python`。脚本使用已安装的 Docker，不安装系统软件，也不询问账号密码。`--check` 只检查 Docker 环境；`--https` 为本次创建的容器传入 HTTPS Cookie 环境变量，重建容器时继续使用此选项，或直接在后台保存该设置。

也可以分步运行：

```bash
docker compose up -d --build
docker compose logs --tail=100 mofufans
```

前台为 `http://localhost:8088/`，后台为 `http://localhost:8088/admin/`。镜像由当前源码构建，不需要另外拉取项目的预编译镜像。

首次打开后台会进入配置引导。复制日志中的一次性安装码，创建管理员并填写站点名称；也可从宿主机的 `data/setup-token` 读取安装码。完成后安装码失效，前台开放。

Compose 挂载 `config/` 和 `data/`。`config/config.json` 是可选文件，默认不需要创建。`data/` 保存自动生成的密钥、后台配置、数据库和本地上传文件，必须持久化。停止服务使用 `docker compose down`，挂载数据会保留；更换宿主机时同时迁移这些目录。

端口默认绑定 `127.0.0.1:8088`。远程服务器可通过 SSH 转发临时访问：

```bash
ssh -L 8088:127.0.0.1:8088 your-user@your-server
```

然后在自己的电脑上打开上述地址。公网访问请配置下文的 HTTPS。

## Linux 源码部署

这是一套完整的生产部署方式，编译完成后由 systemd 管理 API，由 Nginx 提供静态页面。

### 1. 准备环境

需要 Go 1.26.11+、Node.js 22.14+ 和 npm。按 [Go 安装文档](https://go.dev/doc/install) 与 [Node.js 下载说明](https://nodejs.org/en/download)安装；go.mod 同时指定了自动工具链。

Debian / Ubuntu 的其他依赖：

```bash
sudo apt-get update
sudo apt-get install -y build-essential ca-certificates python3 nginx
go version
node --version
npm --version
```

SQLite 使用 CGO，构建时需要 C 编译器。构建阶段还需要联网下载 Go / npm 依赖；小内存服务器可以使用下一节的构建包方式。

### 2. 编译与打包

```bash
python3 scripts/build-release.py
```

脚本依次执行两端的 `npm ci`、生产构建和 `CGO_ENABLED=1 go build`，输出：

```text
dist/mofufans/
├── mofufans                    # Go 可执行文件
├── web/frontend/               # 前台静态页面
├── web/admin/                  # 后台静态页面
├── config/config.example.json  # 配置模板
├── scripts/init-config.py      # 可选的预配置工具
├── deploy/                     # Nginx、systemd、Caddy 模板
├── doc/                        # 部署与配置说明
└── data/                       # 数据目录
```

部署包只收集程序、静态产物、模板和文档，不复制源码目录中的实际配置或数据。已有输出目录时脚本会退出；再次构建可指定新目录：

```bash
python3 scripts/build-release.py --output dist/mofufans-next
```

如需手动执行编译步骤，见[开发指南](DEVELOPMENT.md#生产构建)。

### 3. 初始化安装目录

以下以**首次安装**到 `/opt/mofufans` 为例。已有安装请按[升级](#升级)操作。

```bash
sudo install -d -m 755 /opt/mofufans
sudo cp -a dist/mofufans/. /opt/mofufans/
id -u mofufans >/dev/null 2>&1 || sudo useradd --system --home-dir /opt/mofufans --shell /usr/sbin/nologin mofufans
sudo chown -R root:root /opt/mofufans
sudo chmod 755 /opt/mofufans
sudo chmod -R a+rX /opt/mofufans/web
sudo chown mofufans:mofufans /opt/mofufans/config /opt/mofufans/data
sudo chmod 700 /opt/mofufans/config /opt/mofufans/data
```

默认不需要创建配置文件。下一步启动 API 后，从服务日志复制安装码，打开后台完成设置。需要 MySQL 等预配置时，可先以 `mofufans` 用户运行 `/opt/mofufans/scripts/init-config.py`，再按[配置参考](CONFIGURATION.md)修改。若用 root 生成配置，须将文件属主调整为 `mofufans:mofufans` 并保持仅其可读写，否则 API 服务无法读取。

### 4. 注册 API 服务

```bash
sudo install -m 644 /opt/mofufans/deploy/mofufans.service /etc/systemd/system/mofufans.service
sudo systemctl daemon-reload
sudo systemctl enable --now mofufans
sudo systemctl status mofufans --no-pager
```

服务以专用用户运行，API 监听 `127.0.0.1:8080`，仅 `data/` 可写。若修改安装目录、API 端口或数据路径，需同时调整 service 文件和 Nginx 配置。查看日志：

```bash
sudo journalctl -u mofufans -n 100 --no-pager
```

### 5. 配置 Nginx

```bash
sudo install -m 644 /opt/mofufans/deploy/nginx-native.conf /etc/nginx/conf.d/mofufans.conf
sudo nginx -t
sudo systemctl enable --now nginx
sudo systemctl reload nginx
curl --fail http://127.0.0.1:8088/api/theme
```

模板是供 `http {}` 内包含的站点配置，适用于 Debian / Ubuntu 默认 Nginx 安装。返回的 JSON 中应有 `code: 200` 和站点主题。

前台和后台分别位于 `http://127.0.0.1:8088/`、`http://127.0.0.1:8088/admin/`。首次进入后台，使用日志中的安装码完成引导。远程访问可使用前述 SSH 转发，或继续配置域名。不要把整个 `/opt/mofufans` 设为静态根目录。

## 构建包部署

从[正式Release](https://github.com/DewittCarol/MofuFans/releases/tag/v0.1.1)下载匹配的Linux AMD64／ARM64运行包，先读[下载与使用说明](QUICKSTART.md)并校验SHA256。官方v0.1.1包基于Ubuntu22.04／glibc2.35构建，目标需要相同架构及glibc2.35或更高；不适用于Alpine等musl系统。也可以在兼容的 Linux 电脑、WSL 或构建服务器上执行 `scripts/build-release.py`，再把 `dist/mofufans/` 上传到目标服务器：

```bash
tar -C dist -czf mofufans-linux.tar.gz mofufans
scp mofufans-linux.tar.gz your-user@your-server:~/
```

在目标服务器的一个空目录里解压：

```bash
mkdir -p ~/mofufans-install
tar -xzf ~/mofufans-linux.tar.gz -C ~/mofufans-install
sudo install -d -m 755 /opt/mofufans
sudo cp -a ~/mofufans-install/mofufans/. /opt/mofufans/
```

然后从上节的创建专用用户和目录权限继续，安装 systemd 与 Nginx 配置，启动后在网页初始化。初始化前打包；不要把已经投入使用的配置和 `data/` 当成公开运行包分发。

构建端与运行端必须匹配操作系统、CPU 架构及 C 运行库，建议使用相同发行版版本。ARM64 构建包不能直接用于 x86_64；在较新 glibc 上编译的程序也可能无法在较老系统运行。目标服务器需要 Nginx、CA 证书和兼容的 C 运行库，不需要 Go、Node.js 或 npm。

## 服务器面板与 NAS

已有支持 Compose 的面板或 NAS 时，可以复用仓库中的 `compose.yaml`：

1. 将完整源码放到设备的持久目录。
2. 通过面板导入 Compose 项目，将工作目录和构建上下文设为这个源码目录。
3. 确认 `./config` 和 `./data` 的挂载均解析到持久目录，再构建和启动。
4. 使用面板的 HTTPS 网站／反向代理功能连接服务。
5. 从容器日志复制安装码，打开 `/admin/` 完成配置。

这要求面板支持从本地 Dockerfile 构建。仅支持填写镜像名的安装页面需要先自行构建并导入镜像。

如果面板的反向代理本身在容器里，`127.0.0.1` 指向代理容器而非宿主机。应使用共享 Docker 网络连接 `mofufans:80`，或配置代理访问宿主机的地址；不要直接照抄宿主机回环地址。

## 域名与 HTTPS

应用默认提供按接口分类的限速、并发控制、失败冷却和临时封禁；随附 Nginx 模板另有每实例API总量100请求/秒、突发200的限制，不按客户端IP区分。公网开放前仍需正确配置 HTTPS、可信代理和持久化目录，阅读[防护范围与恢复方式](SECURITY.md)。基本应用保护不依赖 Fail2ban，但 SSH、网络带宽和 CDN 等服务器／网络层防护仍需独立维护，已有 Fail2ban 建议保留。

应用默认只信任回环代理，拒绝 `0.0.0.0/0`、`::/0` 等全地址 CIDR。Docker／面板／CDN 场景还需按实际代理链路设置 `server.trusted_proxies`，核对不同来源是否被识别为各自的客户端 IP；不要把网桥或 CDN 出口误当所有访客的真实 IP，也不要信任任意转发请求头。配置方式见[可信代理说明](SECURITY.md#管理员与会话)。

容器和原生部署默认都在宿主机 `127.0.0.1:8088` 提供完整站点，域名代理指向这里即可。

**使用已有 Nginx 或服务器面板**：创建自己的 HTTPS 站点，将请求代理到 `http://127.0.0.1:8088`。保留原始 Host，传递转发地址和协议，开启 WebSocket；请求体上限至少 40 MB，上传与读取超时建议 900 秒，关闭请求／响应代理缓冲。已有 Nginx HTTPS server 内可以使用：

```nginx
client_max_body_size 40m;
location / {
    proxy_pass http://127.0.0.1:8088;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_request_buffering off;
    proxy_buffering off;
    proxy_read_timeout 900s;
    proxy_send_timeout 900s;
}
```

证书、`server_name` 和 HTTPS 监听由已有站点配置管理。上述外层片段仅提供基础转发，应用内防护和内层模板的总量限制继续生效；Docker、独立代理或 CDN 链路仍需按前述说明核对可信代理配置。

**使用 Caddy**：仓库提供 [deploy/Caddyfile](../deploy/Caddyfile)。将 `fans.example.com` 替换成自己的域名，并把 DNS 指向服务器，再按 [Caddy 文档](https://caddyserver.com/docs/running)启用配置。Caddy 需要使用公网 80／443；已有服务占用这些端口时，应复用现有 HTTPS 代理或先规划端口，不要直接启动两个代理争用端口。

启用 HTTPS 后，在后台「系统设置 → 部署与集成」开启「仅通过 HTTPS 传输登录 Cookie」，保存后立即生效。首次引导通过 HTTPS 打开时，会默认勾选此项。也可以使用 `security.secure_cookie` 或环境变量预设。同域部署时 `server.allowed_origins` 保持空数组即可。

## 备份

需要保存实际配置、数据库以及本地文件；OSS 模式另行备份 Bucket。密钥必须与数据库一起保留，重新生成密钥会导致原有加密数据无法解密。

SQLite 最简单的备份方式是短暂停服后打包。Docker 部署在源码目录执行：

```bash
docker compose down
tar -czf "../mofufans-backup-$(date +%Y%m%d-%H%M%S).tar.gz" config data
docker compose up -d
```

原生部署先 `sudo systemctl stop mofufans`，备份 `/opt/mofufans/config/` 和 `/opt/mofufans/data/`，再启动服务。特别要保留 `data/instance.json` 中的密钥及 `data/settings.json` 中的后台配置。运行中的 SQLite 请使用 backup API，不要只拷贝主数据库文件。MySQL 使用相应的数据库备份工具。

## 升级

先备份，再更新程序；升级时启动会自动迁移数据库结构。

- **Docker**：更新源码，保留 `config/config.json` 和 `data/`，执行 `docker compose up -d --build`。
- **原生部署**：把新版构建到新目录，停掉 API，保留旧程序和静态文件副本，仅替换 `/opt/mofufans/mofufans` 与 `web/`，保留原配置、数据和实例密钥（不得重新生成），再启动 API。新增配置项参考新版示例合并。
- 数据库迁移后仅换回旧程序可能无法运行。升级后已产生新数据时，不能直接用旧整库覆盖，详见[升级与回退](UPGRADING.md)。

目前按单 API 实例运行，上传和队列的部分互斥依赖进程内状态；防护状态文件的操作系统锁也会拒绝同一目录被多个API同时使用。不要仅复制多个容器并共用数据库就当成高可用部署。

## 常见部署问题

| 现象 | 检查位置 |
| --- | --- |
| API 提示配置读取失败 | 默认不需要配置文件；若指定了 `MOFUFANS_CONFIG`，检查该路径是否存在、可读且 JSON 有效 |
| 首次引导无法确认安装码 | 读取当前实例的 `data/setup-token` 或启动日志；已初始化的实例不能再次认领 |
| 提示已有数据库缺少密钥 | 恢复原配置或备份中的 `data/instance.json`，不要生成新密钥替代 |
| SQLite 报 CGO 错误 | 是否安装 C 编译器并使用 `CGO_ENABLED=1` 构建 |
| 本机可访问，公网打不开 | 服务默认绑定回环；检查域名代理、防火墙和 DNS |
| 后台空白或静态文件 404 | Nginx 是否保留 `/admin/`，后台产物是否在 `web/admin/` |
| HTTP 下登录后仍未登录 | `secure_cookie: true` 时请使用 HTTPS |
| 上传失败／413／超时 | 检查内外层代理的请求体上限和超时，确认磁盘／OSS 权限 |
| 新构建提示输出目录已存在 | 指定新的 `--output`；不要覆盖正在运行的部署目录 |

真实 OSS 权限需用部署者自己的凭据检查；B 站私信使用自己的主播扫码会话，只向自己控制的测试 UID 验证，不向真实粉丝发测试消息。系统不会为 OSS 失败静默切回本地；切换存储前先迁移文件，详见[配置参考](CONFIGURATION.md)。

# v0.1.0 下载与使用说明

[返回 README](../README.md) · [完整部署](DEPLOYMENT.md) · [上线安全检查](SECURITY-OPERATIONS.md)

下载页面：https://github.com/DewittCarol/MofuFans/releases/tag/v0.1.0

## 选择下载文件

| 文件 | 适用场景 |
| --- | --- |
| `mofufans-v0.1.0-linux-amd64.tar.gz` | 64位Intel／AMD Linux服务器，`uname -m` 为 `x86_64` |
| `mofufans-v0.1.0-linux-arm64.tar.gz` | 64位ARM Linux服务器，`uname -m` 为 `aarch64` 或 `arm64` |
| `MofuFans-v0.1.0-source.zip` | 修改源码，或用Docker Compose部署；Windows／macOS也使用此方式 |
| `QUICKSTART.zh-CN.md` | 本使用说明，可单独下载阅读 |
| `SHA256SUMS` | 上述下载附件的SHA256校验值 |

原生运行包已包含Go程序、前后台静态页面、部署模板和全部使用／安全文档，不需要安装Go或Node.js。构建基于Ubuntu22.04／glibc2.35，运行端需要**Linux、对应64位架构、glibc2.35或更高、Nginx和CA证书**；建议Ubuntu22.04及更新LTS，或Debian12及更新版本。文件名中的架构是服务器操作系统架构，不能仅看CPU型号。Alpine等musl系统、32位系统和Windows／macOS不能直接运行这些Linux二进制；请使用合适的Docker环境或自行适配构建。

## 校验下载

将选择的包与`SHA256SUMS`放在同一目录。Linux下执行：

```bash
sha256sum --ignore-missing -c SHA256SUMS
```

应看到已下载文件为`OK`，校验失败时不要继续安装。Windows可用PowerShell的`Get-FileHash -Algorithm SHA256 文件名`与清单逐项比较。请从本仓库正式Release下载，不使用陌生人提供的带预设账号或配置的数据包。

## 原生Linux全新安装

1. 检查`uname -m`和`getconf GNU_LIBC_VERSION`，选择匹配包，解压到一个空目录。例如AMD64：

   ```bash
   mkdir -p ~/mofufans-install
   tar -xzf mofufans-v0.1.0-linux-amd64.tar.gz -C ~/mofufans-install
   cd ~/mofufans-install/mofufans
   cat BUILD.json
   ```

2. 在Debian／Ubuntu安装运行依赖：

   ```bash
   sudo apt-get update
   sudo apt-get install -y nginx ca-certificates
   ```

3. 阅读包内`README.md`，按[部署指南的安装目录步骤](DEPLOYMENT.md#3-初始化安装目录)将**整个运行包**放入`/opt/mofufans`，创建专用服务用户并设置权限。随后安装包内的systemd和Nginx模板，执行`nginx -t`成功后再重载。已经解压的包不需要再次执行源码编译或复制不存在的`dist/`目录；以实际路径为准。

4. 首次启动没有默认管理员密码。从`sudo journalctl -u mofufans -n 100 --no-pager`或服务器的`/opt/mofufans/data/setup-token`读取本实例安装码，经只供自己访问的通道进入后台完成认领。

5. 模板默认前台`http://127.0.0.1:8088/`，后台`http://127.0.0.1:8088/admin/`。远程电脑可使用：

   ```bash
   ssh -L 8088:127.0.0.1:8088 your-user@your-server
   ```

   然后在自己电脑访问上述地址。不要把`127.0.0.1`原样发给其他人作为公网地址。

6. 完成初始化后，设置自己的主题和内容。要向粉丝开放时，按[安全指南](SECURITY-OPERATIONS.md)配置自己的域名、有效HTTPS、可信代理、备份和服务器入口。先通过HTTPS打开后台再开启HTTPS Cookie；不要在HTTP转发调试时提前开启。

若目标位置已有网站，请停止“全新安装”流程，按[升级说明](UPGRADING.md)先备份和演练，不用新包覆盖原配置或`data/`，不重新生成原实例密钥。

## Docker／面板／NAS

下载并解压源码包，进入包含`compose.yaml`的目录，确认Docker Engine／Desktop和Compose可用，再执行：

```bash
docker compose up -d --build
docker compose logs --tail=100 mofufans
```

此方案在镜像内编译，首次启动需要联网下载构建依赖，会比原生运行包慢；不要求宿主机安装Go或Node.js。默认宿主机只监听`127.0.0.1:8088`，之后的认领、HTTPS和安全检查与上面一致。面板／NAS的挂载目录和网络转发配置见[完整部署指南](DEPLOYMENT.md#服务器面板与-nas)。

## 配置与功能边界

站名、头像、模块图标、配色、背景、表情和备案信息可在后台配置。新安装没有主播名单、地址、图片或礼物文件，需要自己录入。

B站扫码、签名验证、私信、AI和OSS按需要使用自己的账号和凭据，受上游接口、网络和风控影响；默认本地SQLite／本地文件可先使用。当前只支持单API实例。桌宠需要自备兼容VRM并配置地址，没有模型上传／动作编辑入口；自定义鼠标需制作并安装套装。完整边界见[功能与适配](FEATURES.md)。

程序与静态页已构建、模拟测试通过，不等于所有MySQL／OSS／B站／AI环境均已实测。首次启用外部集成请用自己控制的测试账号验证，尤其不要直接对正式名单测试群发。

## 演示与反馈

前台：https://mofufans-demo.dewitt.work/

后台：https://mofufans-demo.dewitt.work/admin/

演示部署使用独立示例数据，前后台均只读，修改、上传、投稿、私信及真实外部服务调用不可用；不提供生产账号或后台密码。自己部署后才使用自己管理的数据与完整操作能力。

使用问题请查看[仓库README的联系方式](https://github.com/DewittCarol/MofuFans#readme)。漏洞不要在公开Issue中附上凭据或用户数据，报告方式见[安全政策](../SECURITY.md)。

# MofuFans 私信填入助手

这是可选工具：把后台选定的通知填入 B 站私信编辑器，**不点击发送、不读取 Cookie 或聊天记录**。不安装时仍可复制通知手动发送；网站内的“直接发私信”队列也不依赖此助手。

仓库提供的是模板，默认仅允许 `example.com`、示例预览域名与本机测试端口。部署者需要配置自己的域名，不会自动连接其他站点。

支持地址收集和电子礼物两类通知，仅允许这两个管理页面发起请求，发送由本人确认。

## 油猴脚本

1. 在 `userscript-header.txt` 的 `@match`、`userscript-bridge.js` 的 `allowed` 列表中填写自己的精确来源，保留 `https://message.bilibili.com`。
2. 在仓库根目录执行 `python3 scripts/build-whisper-helper.py`（Windows 使用 `python`），重新构建管理端并部署。
3. 用 Tampermonkey 等脚本管理器安装后台提供的脚本。刷新后台及 B 站页面，再检测助手。

也可以先下载合并后的脚本，在脚本管理器中同时修改 `@match` 和 `allowed` 后保存。来源包含协议和非标准端口，不要把白名单改成任意网站。

## Chrome / Edge 扩展

1. 修改 `manifest.json` 中的 `externally_connectable.matches`，以及 `background.js` 中的 `allowedOrigins`。
2. 加载解压后的扩展目录，取得浏览器显示的扩展 ID。
3. 在仓库根目录运行 `python3 scripts/build-whisper-helper.py`（Windows 使用 `python`），生成最新扩展 ZIP。
4. 设置公开环境变量 `NEXT_PUBLIC_WHISPER_EXTENSION_ID` 后重新构建并部署管理端。该 ID 不是密钥。换电脑或重新打包后 ID 可能变化，一般部署建议使用油猴方案。

## 使用与限制

在“地址收集”或“电子礼物”预览通知，点击“打开并自动填入”，核对 UID、内容后手动发送，再回后台标记。每条通知最多 500 字；已有草稿不会被覆盖。

油猴通过一次性任务临时传递当前通知，处理完成即删除，过期任务在后续运行时清理。不要同时安装两种助手。B 站编辑器变化或浏览器权限限制可能影响填入，可随时使用复制按钮。

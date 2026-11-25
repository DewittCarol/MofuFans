# 开发与贡献

[返回 README](../README.md) · [部署指南](DEPLOYMENT.md)

## 环境

- Go 1.26.11+，以及 SQLite CGO 所需的 C 编译器（go.mod 指定自动工具链）。
- Node.js 22.14+ 与 npm。
- Python 3.9+，用于配置初始化、部署辅助和发布检查。

Linux 可安装 `build-essential`。Windows 建议在 WSL 内运行完整开发环境。

## 本地开发

```bash
npm ci --prefix frontend
npm ci --prefix admin
go run ./cmd
```

另开两个终端，分别执行：

```bash
npm run dev --prefix frontend
```

```bash
npm run dev --prefix admin -- -p 3001
```

前台 `http://localhost:3000/`，后台 `http://localhost:3001/admin/`，API 监听 `127.0.0.1:8080`。从 API 日志复制安装码，在后台完成首次引导。开发服务器将 API 和图片请求代理到后端。使用单独的开发数据目录，不要连接实际站点数据库。

## 生产构建

一键构建部署目录：

```bash
python3 scripts/build-release.py
```

也可手动执行底层构建命令：

```bash
npm ci --prefix frontend
npm ci --prefix admin
npm run build --prefix frontend
npm run build --prefix admin
CGO_ENABLED=1 go build -trimpath -o mofufans ./cmd
```

两端产物为 `frontend/out/` 和 `admin/out/`。将它们放入 Nginx 的前后台目录，Go 程序仅提供 API。生产环境不需要 `next start`。完整进程管理与代理配置见[部署指南](DEPLOYMENT.md)。

构建两端时需要保留 `shared/`、`internal/theme/default.json` 和 `internal/database/emoji-defaults.json`，不要只复制单个前端目录。

## 项目结构

```text
frontend/   粉丝前台（Next.js / React / Tailwind CSS）
admin/      管理后台（Next.js / React / Tailwind CSS）
shared/     共享主题配置与组件模板
cmd/        Go 启动入口和配置校验
internal/   API、数据库、存储、B 站接口及业务逻辑
deploy/     原生部署与 HTTPS 配置模板
scripts/    初始化、构建、部署和检查工具
doc/        使用与开发文档
```

## 检查

```bash
go test ./cmd/... ./internal/... ./pkg/...
go build -o mofufans ./cmd
node scripts/test-clients.mjs
python3 scripts/smoke-test.py --binary ./mofufans
python3 scripts/smoke-onboarding.py --binary ./mofufans
python3 -m unittest discover -s scripts -p 'test_deployment.py'
npm run build --prefix frontend
npm run build --prefix admin
python3 scripts/check-release.py
```

冒烟检查使用临时配置和数据库，验证首次初始化、管理员登录与路由。安装 Nginx 并生成静态产物后，可加 `--nginx` 检查代理和页面。测试中使用模拟上游，不发送真实私信。

CI按PR目标提交或本次推送前的祖先提交构建旧API，再演练合成数据升级；首次推送或重建历史后没有可用前序基线时，改为检查同版本重启与持久数据，不将它称为跨版本升级。需要验证更早版本时，自行构建受控旧二进制，再运行 `python3 scripts/smoke-upgrade.py --old-binary /path/to/old-api --binary /path/to/new-api`。脚本通过网页初始化接口创建合成管理员，默认验证合法绑定会话继续有效；只有旧二进制使用旧式未绑定会话时，才额外传入 `--expect-legacy-session-revoked` 验证其被撤销。不依赖已移除的旧仓库提交，也不使用真实业务库做CI。

依赖检查可运行 `npm audit --prefix frontend`、`npm audit --prefix admin` 和 `govulncheck ./...`。请保留锁文件并在更新后重跑构建与回归。生产镜像不包含 Node / ESLint。

## 提交改进

报告问题时写明部署方式、复现步骤、期望行为及脱敏日志。修改功能请覆盖相关测试，修改界面请构建两端并检查实际页面。较大的功能先通过 Issue 讨论范围。

配置、Cookie、名单、地址和上传文件不应进入提交。`scripts/check-release.py` 可以辅助检查常见敏感格式和运行文件，但不能替代人工确认。许可证与原作者署名请完整保留。

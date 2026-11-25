# MofuFans运行包：给维护助手

此目录为预编译Linux运行包，包含程序、前后台静态页、部署模板与文档，不含完整Go／Next.js源码。不要在这里直接运行npm安装或Go构建；需要开发时，到官方源码仓库下载与BUILD.json版本／提交一致的源码。

- 开始前读README.md、doc/DEPLOYMENT.md、doc/SECURITY-OPERATIONS.md、doc/SECURITY.md与doc/UPGRADING.md。
- 先核对Linux架构／glibc、实际目录、服务用户、监听端口、代理链、可恢复管理入口与备份；示例路径不能代替实例盘点。
- 新安装从仅自己可访问的通道完成一次性认领。公网开放前完成安全指南的验收，逐项报告已验证／未验证／不适用及脱敏证据。
- 升级保留原配置、instance.json密钥、数据库、附件和security状态，不覆盖后续业务数据，不重新生成旧实例密钥。运行中SQLite用backup API或停服备份。
- 不公开凭据或个人信息，不关闭鉴权／TLS验证或用chmod 777解决问题；不把data、配置、日志或备份映射为公开静态文件。
- 只支持单API实例。未经所有者授权，不停正式服务、改密、删除数据或对真实粉丝发送测试私信；离线验证与真实外部集成验收分开。

源码与问题反馈：https://github.com/DewittCarol/MofuFans

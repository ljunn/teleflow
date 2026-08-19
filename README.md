# Teleflow

Teleflow 是面向单一所有者的 Telegram 多账号自动化和私域运营工具。系统托管多个工作账号，并通过机器人把客户消息中转到所有者正在使用的 Telegram 主账号。

## 当前状态

项目处于基础设施里程碑，已经包含：

- Go 单体服务和 Vue 3 中文管理端
- SQLite WAL、内嵌数据库迁移和基础数据表
- 健康检查、系统信息、运行概览和 GitHub Release 检查 API
- 默认管理员登录、登录后修改密码和会话保护
- Telegram 手机号、验证码和可选 2FA 授权流程，Session 使用 AES-GCM 加密后持久保存
- 批量导入“手机号 + 取码链接”、后台自动取码登录和账号存活状态检测
- 账号矩阵、采集任务、营销任务和主号中转的持久化管理
- Telegram 与 Relay Bot 运行能力检测和安全等待状态
- GitHub Release 检查、校验下载和网页在线升级
- 前端资源嵌入 Go 二进制
- GitHub Actions、GoReleaser、GHCR 多架构镜像
- systemd 服务和一行安装脚本

Telegram 持续在线连接、采集执行器、营销发送器和机器人消息中转执行器将在后续里程碑实现。新实例首次访问时使用默认管理员密码 `admin`，登录后应立即在“系统设置”中修改密码；已有实例的密码不会在升级时重置。这里的管理员登录与 Telegram 账号登录是两个独立流程。

管理端已经提供账号矩阵、采集任务、营销任务和主号中转配置的持久化控制面。真实连接 Telegram 前，需要在 `/etc/teleflow/teleflow.env` 配置：

```bash
TELEFLOW_TELEGRAM_API_ID=123456
TELEFLOW_TELEGRAM_API_HASH=your_api_hash
TELEFLOW_RELAY_BOT_TOKEN=123456:your_bot_token
```

API ID/hash 是 Telegram 为客户端应用签发的凭据，必须由管理员在 <https://my.telegram.org/apps> 创建应用后取得，不能由 Teleflow 自动生成。Bot Token 需要从 BotFather 获取。配置 API 后，批量导入带取码链接的账号会自动排队登录；登录遇到两步验证时会停在 2FA 输入状态。未配置 API 时，导入仍会保存账号，但会明确记录阻塞原因，不再静默显示为“待登录”。

账号矩阵支持每行一个账号的批量清单，例如 `+10000000000----https://vendor.example/code/GetHTML`。也兼容 `|`、Tab、逗号和分号分隔。取码链接使用实例 Session Key 加密保存，账号列表和 API 响应不会返回完整链接。自动登录会从结构化页面字段读取新验证码和可选 2FA，在短暂返回旧码时有界重试；完成后可执行单账号或批量存活检测，区分在线、受限、会话失效、封禁和连接失败。

## 在线升级

使用 `deploy/install.sh` 安装的 systemd 实例可以在管理端检查并安装 GitHub Release。升级器会下载当前系统和架构对应的压缩包，使用 `checksums.txt` 完成 SHA-256 校验，保留上一版二进制为 `/opt/teleflow/teleflow.previous`，原子替换当前二进制并退出，由 systemd 自动拉起新版本。

从不含在线升级功能的早期版本迁移时，需要先重新执行一次安装脚本，以更新二进制目录权限和 systemd 沙箱配置；此后的版本可以直接从网页升级。容器部署应继续通过拉取新镜像升级，不在容器内替换二进制。

## 本地开发

要求 Go 1.24+、Node.js 22+。

```bash
cd web
npm install
npm run build
cd ..
go run ./cmd/teleflow
```

访问 <http://localhost:8080>。

## 一行安装

发布仓库确定后，将下方地址中的组织名替换为实际 GitHub 组织：

```bash
curl -fsSL https://raw.githubusercontent.com/ljunn/teleflow/main/deploy/install.sh | sudo bash
```

也可以显式指定仓库和版本：

```bash
curl -fsSL https://raw.githubusercontent.com/ljunn/teleflow/main/deploy/install.sh | \
  sudo TELEFLOW_GITHUB_REPOSITORY=ljunn/teleflow TELEFLOW_VERSION=v0.1.0 bash
```

## 发布

推送符合语义化版本的 Tag 即可触发发布：

```bash
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions 会发布 Linux、macOS、Windows 的 amd64/arm64 压缩包、`checksums.txt` 和 GHCR 多架构镜像。

# Teleflow

Teleflow 是面向单一所有者的 Telegram 多账号自动化和私域运营工具。系统托管多个工作账号，并通过机器人把客户消息中转到所有者正在使用的 Telegram 主账号。

## 当前状态

项目处于基础设施里程碑，已经包含：

- Go 单体服务和 Vue 3 中文管理端
- SQLite WAL、内嵌数据库迁移和基础数据表
- 健康检查、系统信息、运行概览和 GitHub Release 检查 API
- 单一管理员初始化、密码登录和会话保护
- GitHub Release 检查、校验下载和网页在线升级
- 前端资源嵌入 Go 二进制
- GitHub Actions、GoReleaser、GHCR 多架构镜像
- systemd 服务和一行安装脚本

Telegram 账号登录、Session 加密和机器人消息中转将在后续里程碑实现。首次访问管理端时需要创建管理员密码；这里的管理员登录与 Telegram 账号登录是两个独立流程。

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

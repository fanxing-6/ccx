# CCX — Claude Code eXecutor

管理多套 Claude Code API 配置，一键切换，即时启动。

所有配置存储在 [Gitee Gist](https://gitee.com/dashboard/codes) 云端，跨机器同步，本地零存储。

## 功能特性

- **云端配置管理** — 配置文件以 JSON 形式存储在 Gitee Gist，任何机器 `ccx init` 即可使用
- **交互式选择器** — 上下键选择 profile，支持搜索过滤
- **完整 CRUD** — 新建、编辑、删除、查看、设置默认配置
- **透明启动** — 通过 `syscall.Exec` 直接替换进程为 `claude`，无子进程开销
- **非 TTY 兼容** — 在 IDE Run Panel 等非终端环境自动回退为数字编号选择

## 安装

### 通过 npm（推荐）

```bash
npm install -g claude-ccx
```

### 通过 GitHub Releases

从 [Releases](https://github.com/fanxing-6/ccx/releases) 下载 `ccx_linux_amd64.tar.gz`：

```bash
curl -Lo ccx.tar.gz https://github.com/fanxing-6/ccx/releases/latest/download/ccx_linux_amd64.tar.gz
tar -xzf ccx.tar.gz
sudo mv ccx /usr/local/bin/
```

### 从源码构建

```bash
git clone https://github.com/fanxing-6/ccx.git
cd ccx
go build -o ccx .
```

## 前置条件

1. **Claude Code CLI** 已安装并在 `$PATH` 中（[安装文档](https://docs.anthropic.com/en/docs/claude-code)）
2. **Gitee 账号** 及 Personal Access Token（需要 Gist 读写权限）
3. **一个 Gitee Gist** 用于存储配置（在 [gitee.com/dashboard/codes](https://gitee.com/dashboard/codes) 创建）

## 快速开始

### 1. 初始化

```bash
ccx init
```

按提示输入：
- Gitee Personal Access Token
- Gitee 用户名
- Gist ID（Gist URL 中 `codes/` 后面的部分）
- Claude 命令名（默认 `claude`）

### 2. 添加配置

```bash
ccx add my-api
```

交互式引导输入 API Token、Base URL、模型等参数。也可用编辑器模式：

```bash
ccx add my-api --editor
```

### 3. 启动

```bash
# 交互式选择
ccx

# 直接指定 profile
ccx my-api

# 危险模式（跳过权限确认）
ccx -d my-api
```

## 命令参考

| 命令 | 说明 |
|------|------|
| `ccx` | 交互式选择 profile 并启动 Claude Code |
| `ccx <name>` | 直接使用指定 profile 启动 |
| `ccx init` | 初始化 Gitee 连接配置 |
| `ccx list` | 列出所有 profile |
| `ccx info <name>` | 查看 profile 详情 |
| `ccx add <name>` | 创建新 profile（交互式引导） |
| `ccx add <name> --editor` | 创建新 profile（编辑器模式） |
| `ccx edit <name>` | 编辑已有 profile |
| `ccx remove <name>` | 删除 profile |
| `ccx default <name>` | 设置默认 profile |

**别名**: `list` → `ls`，`remove` → `rm`

**全局 Flag**: `-d` / `--dangerous` — 为 claude 追加 `--dangerously-skip-permissions`

## 配置结构

### Profile（存储在 Gitee Gist）

每个 profile 是一个 `settings-<name>.json` 文件，内容即 `claude --settings` 接受的 JSON：

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-xxx",
    "ANTHROPIC_BASE_URL": "https://api.example.com/v1",
    "ANTHROPIC_MODEL": "claude-sonnet-4-20250514",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "",
    "API_TIMEOUT_MS": "600000"
  }
}
```

### 本地配置（`~/.config/ccx/config.json`）

仅存储 Gitee 连接信息，不含任何 profile 数据：

```json
{
  "gitee_token": "your-token",
  "gist_id": "your-gist-id",
  "gist_owner": "your-username",
  "claude_command": "claude",
  "default_profile": "my-api"
}
```

## 交互式界面

```
选择 Claude Code 配置:
  ▸ volc                api.volc.com/v1           [claude-sonnet-4-20250514]
    openai              api.openai.com/v1         [gpt-4]
    local               localhost:8080
    ⚙ 配置管理
```

选择「⚙ 配置管理」进入二级菜单，可新建、修改、删除配置或设置默认值，操作完成后自动返回。

## 适用场景

- 有多个 API 供应商（官方、中转、自建）需要频繁切换
- 需要在不同机器间共享 Claude Code 配置
- 希望用不同模型配置应对不同任务

## 卸载

```bash
# npm 方式
npm uninstall -g claude-ccx

# 手动安装方式
rm /usr/local/bin/ccx
```

卸载后本地配置文件不会自动删除（含 Gitee Token），如需彻底清理：

```bash
rm -rf ~/.config/ccx
```

## 系统要求

- Linux amd64
- Node.js 14+（仅 npm 安装方式需要）

## License

MIT

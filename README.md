# CCX

`ccx` 用于管理多套 Claude Code 配置并快速切换启动。

配置存储在 **Gitee Gist**，本地仅保存 Gist 连接信息；支持两种运行模式：
- `anthropic`：直连 Claude 兼容接口
- `openai`：通过本地代理把 Claude Messages 协议转换到 OpenAI/Codex `POST /v1/responses`

## 功能特性

- 云端配置管理：profile 存储在 Gitee Gist（`settings-<name>.json`）
- 交互式启动：支持默认配置、非 TTY 数字回退
- 配置 CRUD：`add / edit / remove / info / list / default`
- OpenAI 模式本地代理：固定上游 `/responses`，不回退 `/chat/completions`
- 模型自动发现：`ccx add` 会请求 `/models`（失败自动回退手填）
- Claude CLI 透传：与 `ccx` 命令不冲突时，自动透传到 `claude`
- 思考强度配置：`OPENAI_REASONING_EFFORT`（严格档位）
- 鉴权安全输出：启动摘要 token 脱敏（前 8 位 + `***`）

## 安装

### 从源码构建

```bash
git clone https://github.com/fanxing-6/ccx.git
cd ccx
go build -o ccx .
```

### npm

```bash
npm install -g claude-ccx
```

## 前置条件

1. 已安装 Claude Code CLI（默认命令名 `claude`）
2. 有可读写 Gitee Gist 的 Token
3. 已创建用于保存 profile 的 Gitee Gist

## 初始化

```bash
ccx init
```

会写入 `~/.config/ccx/config.json`（不包含 profile）：

```json
{
  "gitee_token": "...",
  "gist_id": "...",
  "gist_owner": "...",
  "claude_command": "claude",
  "default_profile": ""
}
```

## 使用方式

### 新建配置

```bash
ccx add <name>
```

交互式创建支持：
- 选择 `api_format`（`anthropic` / `openai`）
- 输入 Base URL（会去掉末尾 `/`，非 `/v1` 仅提示不拦截）
- 自动拉取模型并分页选择（失败回退手填）
- `openai` 模式下可配置 `OPENAI_REASONING_EFFORT`

编辑器模式：

```bash
ccx add <name> --editor
```

### 启动

```bash
# 交互式选择 profile
ccx

# 直接指定 profile
ccx <name>

# 危险模式
ccx -d <name>
```

### 管理

```bash
ccx list
ccx info <name>
ccx edit <name>
ccx remove <name>
ccx default <name>
```

## OpenAI 代理模式（重点）

当 profile 的 `api_format` 为 `openai` 时：

1. `ccx` 启动本地代理（`127.0.0.1:随机端口`）
2. Claude Code 请求本地 `/v1/messages`
3. 代理转换后转发到上游 `POST <ANTHROPIC_BASE_URL>/responses`
4. 上游响应再转换回 Anthropic 事件返回给 Claude Code

说明：
- 当前设计仅支持 `/v1/responses` 路径
- 上游请求会同时带：
  - `Authorization: Bearer <token>`
  - `X-Api-Key: <token>`

## 思考强度（OPENAI_REASONING_EFFORT）

仅在 `api_format=openai` 的本地代理路径生效。

可选值：
- `none`
- `auto`
- `minimal`
- `low`
- `medium`
- `high`
- `xhigh`

行为规则：
- profile 配置非法值：启动时 fail-fast 报错
- 若请求已显式带 `thinking`：代理不覆盖
- 若请求未带 `thinking`：代理按档位注入默认 `thinking`

映射（与 CLIProxyAPI 语义一致）：
- `none` -> `thinking.type=disabled`
- `auto` -> `thinking.type=enabled, budget_tokens=-1`
- `minimal` -> `512`
- `low` -> `1024`
- `medium` -> `8192`
- `high` -> `24576`
- `xhigh` -> `32768`

## 命令透传

当参数不属于 `ccx` 自有命令，且符合透传规则时，`ccx` 会直接执行 `claude ...`。

示例：

```bash
ccx auth status
ccx update
ccx -- -p "hello"
ccx -d auth status
```

内置透传顶级命令：
- `auth`
- `update`
- `mcp`
- `agents`
- `remote-control`

## Profile 示例

### anthropic（默认）

```json
{
  "env": {
    "ANTHROPIC_API_KEY": "sk-...",
    "ANTHROPIC_AUTH_TOKEN": "sk-...",
    "ANTHROPIC_BASE_URL": "https://api.example.com/v1",
    "ANTHROPIC_MODEL": "claude-sonnet-4-20250514",
    "API_TIMEOUT_MS": "600000"
  }
}
```

### openai（responses 代理）

```json
{
  "api_format": "openai",
  "env": {
    "ANTHROPIC_API_KEY": "sk-...",
    "ANTHROPIC_AUTH_TOKEN": "sk-...",
    "ANTHROPIC_BASE_URL": "https://api.linkflow.run/v1",
    "ANTHROPIC_MODEL": "gpt-5.3-codex",
    "OPENAI_REASONING_EFFORT": "high",
    "API_TIMEOUT_MS": "600000"
  }
}
```

## 运行与测试

```bash
go test ./...
go build -o ccx .
```

## License

MIT

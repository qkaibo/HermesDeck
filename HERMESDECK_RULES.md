# HermesDeck — 项目规则与设计决策

## 铁律（不可违背）

### 代码隔离

- **所有代码必须在 `~/HermesDeck/` 目录下**
- 禁止 symlink 指向外部目录（PilotDeck、Hermes Agent 等）
- 禁止修改 `~/HermesDeck/` 以外任何目录的文件
- PilotDeck 源码完整拷贝在 `~/HermesDeck/src/`（20+ 个目录，~13MB），零外部引用

### 端口隔离

| 服务 | 端口 | 说明 |
|------|------|------|
| HermesDeck WebSocket Gateway | 28788 | 替换原 PilotDeck Gateway（18788） |
| HermesDeck UI | **28789** | **和 PilotDeck 的 18789 不冲突** |
| HermesDeck Go gRPC | 29551 | 内部通信 |
| Python Sidecar | 29552 | Hermes Agent 运行时 |

PilotDeck 原服务端口（18788/18789）完全保留，不可占用。

### 修改前请示

删除/覆盖任何文件前，必须先请示用户。

---

## 外部依赖（不拷贝，只引用路径）

| 依赖 | 路径 | 说明 |
|------|------|------|
| Hermes Agent | `~/.hermes/hermes-agent/` | Python venv（3.11）+ 模块，升级自动生效 |
| Node.js | `~/.n/bin/node` | v22.22.3，UI 需要 `node:sqlite` |
| Node 依赖 | `~/HermesDeck/node_modules/` | 从 PilotDeck 拷贝，677MB |

---

## 项目结构

```
~/HermesDeck/
├── ui/                       ← PilotDeck UI（Express + React）
│   ├── server/               后端源码
│   ├── dist/                 React 构建产物
│   └── shared/               共享模块（modelConstants.js 等）
├── src/
│   ├── go/                   HermesDeck Go Runtime
│   ├── python/               HermesDeck Python Sidecar
│   ├── agent/...             ← PilotDeck src/ 完整拷贝
│   ├── gateway/...
│   ├── cli/...
│   └── ...（20+ 个目录）
├── node_modules/             全部依赖
├── bin/hermesdeck            编译后的 Go 二进制
└── scripts/run-dev.sh        启动脚本
```

---

## 架构

```
用户浏览器 → HermesDeck UI (28789)
              ↓ WebSocket (PilotDeck Gateway 协议)
       Node.js Gateway (28788) ← ~/HermesDeck/dist/ + ~/HermesDeck/src/hermesdeck-gateway.mjs
              │
              ├─ 27 个 RPC：skill_* / cron_* / project_* / ...
              │   → Gateway 原生实现（PilotDeck 原版代码）
              │
              └─ submit_turn（聊天对话）
                  → Python Sidecar (29552) → Hermes AIAgent
```

### 三层

1. **Node.js Gateway**（`dist/` 编译产物）— WebSocket 服务器
   - 处理所有非聊天的 RPC（技能、项目、cron、权限等）
   - 聊天请求转发给 Python Sidecar
   - 端口 28788

2. **Python Sidecar** — gRPC 服务端
   - 接收 Gateway 的聊天请求（ProcessMessage）
   - 使用 Hermes Agent 的 AIAgent 处理对话
   - 端口 29552

3. **UI Express Server** — 前端页面
   - `PILOTDECK_GATEWAY_URL=ws://127.0.0.1:28788/ws`
   - 端口 28789

---

## 启动方式

```bash
bash ~/HermesDeck/scripts/run-dev.sh
```

启动顺序：Python Sidecar → Go Runtime → UI Server

---

## 数据目录

HermesDeck 使用独立数据目录 `~/.hermesdeck/`，与 PilotDeck 的 `~/.pilotdeck/` 完全隔离。

```
~/.hermesdeck/
├── projects/          项目数据
├── pilotdeck.yaml     模型配置
├── server-token       Gateway 认证令牌
└── auth.db            用户认证
```

通过 `PILOT_HOME=~/.hermesdeck` 环境变量控制，所有服务启动时自动设置。

## Gateway RPC 实现状态

30 个 RPC 方法，16 个有真实实现，14 个返回 stub（空数据，前端不报错）：

### 已实现（真实逻辑）
`submit_turn` · `abort_turn` · `new_session` · `resume_session` · `close_session` · `list_sessions` · `describe_server` · `list_projects` · `skill_list` · `skill_read` · `skill_write` · `skill_create` · `skill_delete` · `skill_validate` · `skill_import` · `skill_scan`

### Stub（返回空值）
`active_turn_snapshot` · `read_session_messages` · `describe_project` · `reload_config` · `cron_create/list/delete/stop/run_now` · `elicitation_respond` · `permission_decide` · `grant_session_permission` · `always_on_apply/rerun_plan`

## 技能系统

### 存储结构
技能是**双作用域**的，存在独立数据目录下：

```
User skills (所有项目共享)
  ~/.hermesdeck/skills/<slug>/SKILL.md

Project skills (仅该项目可见)
  <projectRoot>/.pilotdeck/skills/<slug>/SKILL.md
```

每个技能是一个目录，内含 `SKILL.md`（YAML frontmatter + Markdown 正文）。

### 数据来源
`skill_list` 从三个来源合并展示（同名去重）：
1. `PILOT_HOME/skills/` — HermesDeck 用户技能
2. `HERMES_HOME/skills/` — Hermes 配置文件技能
3. `~/.hermes/hermes-agent/skills/` — Hermes 内置技能

## 未完成的缺口
2. **systemd 服务** — 目前只能手动启动
3. **Go 的 `list_projects` 返回字段** — 只返回了 name/displayName/fullPath/path，缺少 sessions/meta 等
4. **端到端自动测试** — 无自动化测试

---

## 决策历史

| 日期 | 决策 |
|------|------|
| 2026-05-29 | 恢复 PilotDeck systemd 服务（18788/18789），解决 bridge 连接问题 |
| 2026-05-29 | HermesDeck 端口从默认改为 18988/19551/19552 |
| 2026-05-29 | 端口改为 2 开头：28788/29551/29552 |
| 2026-05-29 | UI 端口改为 28789（隔离 PilotDeck 的 18789） |
| 2026-05-29 | Python sidecar 改用 Hermes venv Python（3.11）而非系统 Python（3.8） |
| 2026-05-29 | agent_wrapper.py 重写：sys.path 设置、config 加载、thread-safe streaming |
| 2026-05-29 | Go handler 加 `list_projects` RPC |
| 2026-05-29 | UI 从 PilotDeck 目录独立搬入 `~/HermesDeck/ui/` |
| 2026-05-29 | 禁止 symlink，PilotDeck 源码完整拷贝进 `~/HermesDeck/src/` |
| 2026-05-29 | HermesDeck 所有代码必须在自身目录下，不可 link、不可改外部文件 |

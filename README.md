> **本仓库已停止维护，后续开发已迁移。**
>
> 最新代码、文档、版本发布和问题反馈请前往 **[cocoyu-workspace/notion-code-bridge](https://github.com/cocoyu-workspace/notion-code-bridge)**。
> 本仓库仅保留历史记录，将归档为只读。请在新仓库提交 Issue / PR，并重新 Star 或 Watch 新仓库。

<div align="center">
  <h1>Notion Code Bridge</h1>
  <p><strong>将 Notion AI 接入本地 Claude Code</strong></p>
  <p>支持使用具备 Notion AI 权限的 Notion Business 订阅，在本地 Claude Code 终端中进行对话、读取文件、修改代码和执行测试。</p>
  <p><strong>简体中文</strong> · <a href="./README_EN.md">English</a></p>
</div>

## 项目介绍

Notion Code Bridge 将 Notion AI 的请求和响应转换为本地编程客户端可用的接口。
使用自己的 Notion Business 账号完成登录后，可以通过本地代理接入 Claude Code。
模型推理发生在 Notion，本地文件读取、编辑和终端命令由你电脑上的 Claude Code 执行。

本项目独立维护，最初 fork 自
[SleepingBag945/notion_manager](https://github.com/SleepingBag945/notion_manager)。
感谢原作者提供的账号池、管理面板和协议代理基础；本项目继续开发 Notion 到本地编程工具的桥接能力。

## 当前能力

- **Claude Code 接入**：提供 Anthropic Messages 兼容接口，配有独立启动器，不覆盖其他提供方的全局配置。
- **本地工具调用**：已实测文件读取、代码编辑、终端运行测试。
- **本地 skills**：发现项目和用户目录中的技能，按需读取原始 `SKILL.md`；原生 `/技能名` 命令尚未验证。
- **账号管理**：保留账号池、额度刷新和 Dashboard。
- **其他客户端**：保留 OpenAI Chat Completions、Responses 等兼容入口。

当前个人 Agent 新协议支持 **Auto 和 Opus 5.5**。实际可用模型和额度取决于账号、工作区和 Notion 当前策略。
开通 Business 不代表本代理已支持指定所有模型，也不代表可以无限调用。

## 快速开始

### 1. 下载并构建

需要 Go 1.26+、已安装的 Claude Code，以及具备 Notion AI 权限的账号。
建议使用你自己的 Notion Business 订阅。

```bash
git clone https://github.com/wycocoyu/notion-code-bridge.git
cd notion-code-bridge
go build -o notion-manager ./cmd/notion-manager
./notion-manager
```

首次启动会生成本地配置。按照终端提示保存管理密码，并打开 Dashboard
（默认 `http://127.0.0.1:8081/dashboard/`，具体端口以配置为准）导入自己的 Notion 登录态。
账号文件和配置包含凭据，请勿提交到 Git 或贴到聊天中。

### 2. 启用新协议

退出首次启动的代理，在已生成的 `config.yaml` 对应段中加入或修改以下字段。
**保留已有 `api_key`、`admin_password` 和其他设置，不要用此片段覆盖整个配置。**

```yaml
server:
  host: 127.0.0.1
  port: "8081"
proxy:
  agent_thread: true
  notion_api_base: https://app.notion.com/api/v3
  default_model: auto
```

### 3. 启动本地 Claude Code

启动器需要 Python 3 和 PyYAML。以下为 macOS / Linux 示例：

```bash
python3 -m venv .venv
. .venv/bin/activate
python -m pip install PyYAML
python claude-notion.py
```

要在其他项目中工作，先进入那个项目目录，再用绝对路径运行启动器：

```bash
cd /path/to/your/project
/path/to/notion-code-bridge/.venv/bin/python /path/to/notion-code-bridge/claude-notion.py
```

启动器会在需要时启动本地代理，并使用当前目录作为 Claude Code 工作目录。
它采用 bare 模式和基础本地工具；插件、hooks、外部 MCP 和自动 `CLAUDE.md` 发现尚未启用。

账号具有权限时，可以显式选择 Opus 5.5：

```bash
python claude-notion.py --model opus-5.5
```

## 会话和使用边界

Claude Code 保存自己的对话历史。每次推理，包括工具执行后的下一次请求，
代理都会新建 Notion 线程，并带上客户端当次提供的历史和工具结果。
这不是与 Notion 网页共用一个会话，网页已有聊天不会自动导入。

- 当前为实验性文本工具桥接，未完整保留原生模型角色语义。
- 新个人 Agent 协议暂不支持图片和文件附件。旧会话中的附件也可能导致后续请求失败。
- 上游回复完成后再转换为 SSE；目前不是逐 token 流式输出。
- 本地超时不等于取消远端推理；重试可能产生另一份回答。
- 长上下文压缩、并发和会话恢复尚未充分验证。

## 更多文档

- [个人 Agent 协议、启动器与验证范围](AGENT_THREAD.md)
- [管理面板及旧协议详细指南](README_CN.md)
- [配置说明](docs/configuration_cn.md)
- [Claude Code 集成](docs/claude-code-integration.md)

## 来源与许可证

本项目 fork 自 [SleepingBag945/notion_manager](https://github.com/SleepingBag945/notion_manager)，
在保留上游贡献与署名的基础上独立开发。
继续采用原项目的 [CC BY-NC-SA 4.0](LICENSE) 许可证，仅限非商业用途。
这里的 Notion Business 指 Notion 的订阅套餐，不改变本项目的许可证条件。

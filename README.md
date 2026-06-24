# Plugin Broker IPC Execution System

一个使用 Go 实现的插件化执行系统。当前项目采用中心化 Broker + 多进程 IPC 架构：主进程负责配置加载、插件注册、依赖校验、请求路由、断路器和执行编排；插件以独立子进程运行，并通过 Unix Domain Socket 进行 JSON 请求/响应通信。

## 功能

- 支持通过 `plugins.json` 启用或禁用插件
- 支持插件依赖声明，并通过拓扑排序计算执行顺序
- 支持重复插件配置检测、缺失依赖检测、禁用依赖检测和循环依赖检测
- 支持插件主动向 Broker 注册元数据，包括插件 ID、版本号、依赖列表、UDS 地址和服务描述
- Broker 注册插件前会先对插件提交的 UDS 地址执行带超时的 Dial 校验
- 支持插件版本更新时的运行时上下文替换，新请求自动路由到新版本
- 支持旧版本插件的活跃请求计数和 10 秒强制回收
- 支持插件启用/关闭状态切换，关闭时不主动断开底层 UDS 连接
- 支持每个插件独立执行超时，默认 30000ms，最大 60000ms
- 支持断路器，插件连续失败 5 次后 Broker 会拒绝继续转发请求
- 单个插件失败、超时或子进程异常不会导致整个 Pipeline 中断
- 输出每个插件的启用状态、执行状态、耗时和错误信息
- 支持 `plugins.json` 配置热加载
- 提供 `uppercase`、`wordcount`、`timestamp` 三个示例插件进程

## 运行

```bash
go run ./cmd/app
```

自定义输入：

```bash
go run ./cmd/app -input "hello plugin system"
```

自定义配置：

```bash
go run ./cmd/app -config plugins.json -input "hello plugin system"
```

监听配置变化并热加载：

```bash
go run ./cmd/app -config plugins.json -input "hello plugin system" -watch
```

`-watch` 模式会持续监听 `plugins.json`。文件变化后，程序会重新读取配置、重建执行计划，并使用新的 Pipeline 再执行一次。

## 插件接口


```go
type Plugin interface {
    Name() string
    Version() string
    Run(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)
}
```

多进程插件通过 `internal/sdk` 启动 UDS JSON 服务，并主动向 Broker 注册。Broker 通过 `NewBrokerPlugin` 将远程插件适配为 Pipeline 可执行节点。

## 配置格式

```json
{
  "plugins": [
    {
      "name": "uppercase",
      "enabled": true,
      "timeout_ms": 1000
    },
    {
      "name": "wordcount",
      "enabled": true,
      "depends_on": ["uppercase"],
      "timeout_ms": 1000
    }
  ]
}
```

字段说明：

- `name`：插件名称，也对应 Broker 中的 PluginID
- `enabled`：是否启用插件
- `depends_on`：当前插件依赖的其他插件名称
- `timeout_ms`：单个插件最大执行时间，最大值为 60000ms

## 新增插件

新增插件推荐使用 `internal/sdk` 创建独立进程。插件进程需要完成两件事：

1. 启动 UDS JSON 服务，处理 Broker 投递的请求
2. 启动后向 Broker 注册插件元数据

示例：

```go
server := sdk.Server{
    PluginID: "example",
    Handler: func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
        data["example"] = true
        return data, nil
    },
}

go server.Serve(ctx, udsPath)

err := sdk.Register(ctx, brokerRegisterPath, core.PluginMeta{
    PluginID:    "example",
    Version:     "1.0.0",
    DependsOn:   []string{"uppercase"},
    UDSPath:     udsPath,
    Description: "example plugin",
})
```

随后在 `plugins.json` 中启用：

```json
{
  "name": "example",
  "enabled": true,
  "depends_on": ["uppercase"]
}
```

## 设计说明

系统当前采用中心化 Broker + 多进程 IPC，而不是把所有插件直接编译进主进程。这样可以让插件崩溃、阻塞或异常退出时不拖垮主程序。

### 插件系统的核心设计思路

插件启动后通过 SDK 主动向 Broker 发起注册请求。Broker 不盲信插件提交的 UDS 地址，而是在修改内部状态前先执行 Dial 校验。只有连接成功、依赖图无环时，插件才会进入核心路由表。

执行时，系统先根据配置生成 `[]ConfiguredPlugin`，再交给 `Pipeline` 顺序执行。Pipeline 节点通过 Broker 远程调用插件进程：

```text
input -> Broker -> uppercase process -> Broker -> wordcount process -> result
```

插件执行失败时不会中断整个流程，后续插件仍会继续执行。失败插件的错误会被记录到 `plugin_results` 中。

### 关键实现选择与取舍说明

- 选择 Broker 集中路由：注册、健康检查、启停、断路器、依赖图和版本切换都由 Broker 统一管理。
- 选择 UDS + JSON：实现简单、跨进程边界清晰，适合作为当前项目的第一版 IPC 协议。
- 注册前强制 Dial：Broker 只有确认插件服务可连接后才写入注册表，避免无效地址污染核心状态。
- 依赖图支持回滚：新插件注册时会先写入依赖图并检测环；失败后恢复旧依赖数据并关闭新连接。
- 版本更新使用运行时上下文替换：新版本注册成功后立即接管新请求，旧版本通过活跃请求计数平滑退出。
- 禁用插件不关闭连接：`enabled=false` 只影响新请求路由，不主动断开 UDS，后续重新启用可以快速恢复。
- 断路器保护下游插件：连续失败 5 次后，Broker 会直接拒绝请求，给插件进程恢复空间。
- 保留 Pipeline 结果模型：继续输出 `data` 和 `plugin_results`，兼容原有执行结果结构。

### 进阶能力设计说明

- 插件热更新：当前支持新版本注册后覆盖核心路由表，旧版本请求自然完成后回收连接。
- 插件热启停：当前支持配置级启用/禁用，不断开底层连接。
- 插件执行超时：当前由 Pipeline 使用 `context.WithTimeout` 控制每个插件的最大执行时间。
- 失败隔离：插件进程异常、请求失败、超时都会记录为失败结果，并继续执行后续插件。
- 断路器：当前支持连续失败阈值保护，后续可以扩展半开探测窗口和恢复策略配置。
- 多语言插件：当前示例插件使用 Go 实现；后续只要遵守 UDS JSON 协议，也可以接入 Python、Node.js、Rust 或其他语言实现的插件。

### 未完成部分的设计想法

- 外部插件配置：当前 `cmd/app` 内置了示例插件进程启动逻辑，后续可在配置中增加 `command`、`args`、`env` 字段。
- 注册鉴权：当前插件注册未做身份认证，后续可加入 token、签名或本地权限校验。
- 更完整的健康检查：当前注册时做 Dial 校验，后续可以增加周期性 ping、心跳和主动摘除。
- 更细粒度资源限制：当前主要限制执行时间，后续可以增加并发数、输出大小、内存和 CPU 限制。
- 更稳健的热加载失败处理：当前配置解析失败仍可能终止 watcher，后续可改为保留旧 Pipeline 并等待下一次配置修复。

## 测试

```bash
go test ./...
```

运行示例：

```bash
go run ./cmd/app -input "hello plugin system"
```

# Plugin-based Execution System

一个使用 Go 实现的插件化执行系统。主程序负责加载配置、管理插件和编排执行流程，业务处理逻辑由插件提供。

## 功能

- 支持通过 `plugins.json` 启用或禁用插件
- 支持插件依赖声明，并自动计算执行顺序
- 支持重复插件配置检测和循环依赖检测
- 支持插件执行超时控制，默认 30000ms，最大 60000ms
- 单个插件失败、超时或 panic 不会导致主程序崩溃
- 输出每个插件的启用状态、执行状态、耗时和错误信息
- 提供 `uppercase`、`wordcount`、`timestamp` 三个示例插件

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

## 插件接口

```go
type Plugin interface {
    Name() string
    Version() string
    Run(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)
}
```

插件通过 `context.Context` 接收取消信号。插件实现中应主动检查 `ctx.Err()`，或在阻塞操作中监听 `ctx.Done()`。

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

- `name`：插件名称
- `enabled`：是否启用插件
- `depends_on`：当前插件依赖的其他插件名称
- `timeout_ms`：单个插件最大执行时间，最大值为 60000ms

## 新增插件

在 `internal/plugins` 中新增一个返回 `core.Plugin` 的函数：

```go
func Example() core.Plugin {
    return core.NewPlugin("example", "1.0.0", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
        if err := ctx.Err(); err != nil {
            return data, err
        }
        data["example"] = true
        return data, nil
    })
}
```

然后在 `RegisterDefaults` 中注册：

```go
_ = manager.Register(Example())
```

最后在 `plugins.json` 中启用：

```json
{
  "name": "example",
  "enabled": true
}
```

## 设计说明

系统没有使用 Go 原生 `.so` 动态插件，原因是该机制在 Windows 环境下支持有限。当前实现使用“插件注册 + 配置驱动”的方式，保留插件化系统的核心能力，同时保证跨平台可运行。

核心模块：

- `internal/core/plugin.go`：插件接口和基础插件实现
- `internal/core/manager.go`：插件注册、依赖校验、拓扑排序
- `internal/core/pipeline.go`：插件流水线执行、超时控制、错误隔离
- `internal/core/config.go`：插件配置读取
- `internal/plugins/plugins.go`：示例插件集合
- `cmd/app/main.go`：命令行入口

## 面试讲解重点

### 1. 插件依赖关系与执行顺序控制

插件可以通过 `depends_on` 声明依赖关系。系统启动时会检查依赖插件是否存在、是否启用，并检测循环依赖。校验通过后，系统使用拓扑排序生成最终执行顺序。

### 2. 插件运行状态管理与执行报告

每个插件都会输出独立执行报告：

```json
{
  "name": "uppercase",
  "version": "1.0.0",
  "enabled": true,
  "status": "success",
  "duration_ms": 1
}
```

状态包括：

- `success`：插件执行成功
- `failed`：插件执行失败、超时或 panic
- `skipped`：插件被禁用，未执行

### 3. 插件失败隔离与超时控制

系统会为每个插件创建带超时的 `context.Context`。插件返回错误、panic 或超过 `timeout_ms` 时，系统会记录失败原因，并继续执行后续插件。

## 测试

```bash
go test ./...
```

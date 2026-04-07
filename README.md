# AIS Go Pkg

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> 企业级 Go Web 开发公共组件库 - 沉淀最佳实践，加速业务开发

## ✨ 核心特性

- 🎯 **接口优先** - 清晰的抽象层，易于扩展和测试
- ⚙️ **配置驱动** - YAML + 环境变量，灵活适配多环境
- 🔌 **零侵入设计** - 不绑定特定框架，按需集成
- 🧪 **高可测试性** - 完整的 Mock 支持和测试工具
- 📦 **开箱即用** - 预配置最佳实践，减少重复工作
- 🚀 **生产就绪** - 经过实战验证的企业级组件

---

## 📦 组件清单

| 组件 | 功能 | 核心依赖 |
|------|------|---------|
| **logger** | 结构化日志 | zap |
| **conf** | 配置管理 | viper |
| **database** | 数据库连接池 | gorm, postgres, mysql |
| **cache** | Redis 客户端 + 分布式锁 | go-redis/v9 |
| **mq** | 消息队列抽象层 | Kafka, RocketMQ |
| **idempotency** | 幂等性检查 | Redis SetNX + TTL |
| **transport** | HTTP/gRPC 服务器 | Fiber v3, gRPC |
| **metrics** | Prometheus 监控 | prometheus/client_golang |
| **middleware** | HTTP 中间件 | CORS、API Key 认证、Auth Header 透传、错误处理、限流等 |
| **errors** | 统一错误处理 | gRPC/HTTP 错误转换 |
| **repository** | 数据仓储模式 | CRUD, 分页, 聚合 |
| **response** | 统一响应格式 | HTTP 响应封装 |
| **validator** | 数据验证 | validator/v10 |
| **shutdown** | 优雅关闭 | 分优先级资源清理 |
| **ulid** | 分布式唯一 ID | oklog/ulid/v2 |

---

## 🚀 快速开始

### 配置文件示例

完整的配置文件示例请参考：
- [config.example.yaml](conf/config.example.yaml) - 包含所有组件的完整配置（HTTP、数据库、Redis、MQ、中间件等）

### 安装

#### 方式一：本地开发（推荐）

```bash
# 在你的项目 go.mod 中添加
replace github.com/aisgo/ais-pkg => ../ais-pkg

require github.com/aisgo/ais-pkg v0.0.0
```

#### 方式二：Git 依赖（正式发布后）

```bash
go get github.com/aisgo/ais-pkg@v1.0.0
```

### 基础示例

#### 方式一：手动集成（适合简单场景）

```go
package main

import (
    "github.com/aisgo/ais-pkg/logger"
    "github.com/aisgo/ais-pkg/validator"
    "go.uber.org/zap"
)

func main() {
    // 1. 初始化日志
    log := logger.NewLogger(logger.Config{
        Level:  "info",
        Format: "console",
    })
    
    // 2. 初始化验证器
    v := validator.New()
    
    type User struct {
        Name string `validate:"required"`
    }
    
    if err := v.Validate(&User{}); err != nil {
        log.Info("validation failed", zap.Any("error", err))
    }

    log.Info("application started")
}
```

#### 方式二：使用 Fx 模块（推荐，适合复杂应用）

```go
package main

import (
    "github.com/aisgo/ais-pkg/cache"
    "github.com/aisgo/ais-pkg/cache/redis"
    "github.com/aisgo/ais-pkg/database/postgres"
    "github.com/aisgo/ais-pkg/logger"
    "github.com/aisgo/ais-pkg/mq"
    "github.com/aisgo/ais-pkg/transport/http"
    "github.com/gofiber/fiber/v3"
    "go.uber.org/fx"
    "gorm.io/gorm"
)

func main() {
    app := fx.New(
        // ================================================================
        // 配置提供
        // ================================================================
        fx.Provide(
            func() logger.Config {
                return logger.Config{Level: "info", Format: "json"}
            },
            func() postgres.Config {
                return postgres.Config{
                    Host:     "localhost",
                    Port:     5432,
                    User:     "user",
                    Password: "pass",
                    DBName:   "mydb",
                }
            },
            func() redis.Config {
                return redis.Config{
                    Host: "localhost",
                    Port: 6379,
                }
            },
            func() *mq.Config {
                return &mq.Config{
                    Type: mq.TypeKafka,
                    Kafka: &mq.KafkaConfig{
                        Brokers: []string{"localhost:9092"},
                    },
                }
            },
            func() http.Config {
                return http.Config{Port: 8080}
            },
            http.NewHTTPServer,
        ),
        
        // ================================================================
        // 导入组件模块
        // ================================================================
        logger.Module,
        postgres.Module,
        cache.Module,
        mq.Module,
        
        // ================================================================
        // 业务逻辑
        // ================================================================
        fx.Invoke(func(
            log *logger.Logger,
            db *gorm.DB,
            fiberApp *fiber.App,
        ) {
            log.Info("application started successfully")
        }),
    )
    
    app.Run()
}
```

---

## 📚 组件详解

### 🪵 Logger - 结构化日志

基于 Zap 的高性能日志组件，支持 JSON 和 Console 格式。

#### 直接使用

```go
import "github.com/aisgo/ais-pkg/logger"

log := logger.NewLogger(logger.Config{
    Level:      "info",        // debug, info, warn, error
    Format:     "json",        // json | console
    Output:     "app.log",     // 可选，默认 stdout
})

log.Info("user login", 
    zap.String("user_id", "123"),
)
```

#### 使用 Fx 模块

```go
import (
    "github.com/aisgo/ais-pkg/logger"
    "go.uber.org/fx"
)

app := fx.New(
    fx.Provide(func() logger.Config {
        return logger.Config{Level: "info", Format: "json"}
    }),
    logger.Module,
    fx.Invoke(func(log *logger.Logger) {
        log.Info("application started")
    }),
)
```

### 🗄️ Database - PostgreSQL / MySQL + GORM

提供数据库连接池、GORM 集成及日志适配。推荐通过 Fx 模块使用。

#### 使用 Fx 模块 (PostgreSQL)

```go
import (
    "github.com/aisgo/ais-pkg/database/postgres"
    "github.com/aisgo/ais-pkg/logger"
    "go.uber.org/fx"
    "gorm.io/gorm"
)

app := fx.New(
    fx.Provide(
        func() logger.Config { return logger.Config{Level: "info"} },
        func() postgres.Config {
            return postgres.Config{
                Host:   "localhost",
                Port:   5432,
                User:   "postgres",
                DBName: "mydb",
            }
        },
    ),
    logger.Module,
    postgres.Module,
    fx.Invoke(func(db *gorm.DB) {
        // 使用 db...
    }),
)
```

#### 使用 Fx 模块 (MySQL)

```go
import (
    "github.com/aisgo/ais-pkg/database/mysql"
    "github.com/aisgo/ais-pkg/logger"
    "go.uber.org/fx"
    "gorm.io/gorm"
)

app := fx.New(
    fx.Provide(
        func() logger.Config { return logger.Config{Level: "info"} },
        func() mysql.Config {
            return mysql.Config{
                Host:   "localhost",
                Port:   3306,
                User:   "root",
                DBName: "mydb",
            }
        },
    ),
    logger.Module,
    mysql.Module,
    fx.Invoke(func(db *gorm.DB) {
        // 使用 db...
    }),
)
```

### 💾 Cache - Redis 客户端

封装 go-redis/v9，提供分布式锁实现。推荐通过 Fx 模块注入 `redis.Clienter` 接口。

#### 使用 Fx 模块

```go
import (
    "context"
    "time"
    "github.com/aisgo/ais-pkg/cache"
    "github.com/aisgo/ais-pkg/cache/redis"
    "github.com/aisgo/ais-pkg/logger"
    "go.uber.org/fx"
)

app := fx.New(
    fx.Provide(
        func() logger.Config { return logger.Config{Level: "info"} },
        func() redis.Config {
            return redis.Config{
                Host:         "localhost",
                Port:         6379,
            }
        },
    ),
    logger.Module,
    cache.Module,  // 严格模式：连接失败时阻塞启动
    fx.Invoke(func(client redis.Clienter, log *logger.Logger) {
        ctx := context.Background()
        _ = client.Set(ctx, "key", "value", time.Hour)
        
        // 分布式锁
        lock := client.NewLock("resource:order:123")
        if err := lock.Acquire(ctx); err == nil {
            defer lock.Release(ctx)
            // 临界区代码
        }
    }),
)
```

> ✅ **接口优先**：通过 Fx 模块注入时推荐使用 `redis.Clienter` 接口，便于 mock/替换实现。
> 
> ✅ **自动续期机制**：默认启用 `AutoExtend: true`，防止长时间任务导致锁提前释放。
>
> ✅ **Optional 模式**：使用 `cache.OptionalModule` 可实现配置缺失或连接失败时返回 nil，不阻塞应用启动。


### 📨 MQ - 消息队列抽象层

统一接口，支持 Kafka 和 RocketMQ 无缝切换。
Kafka Consumer 默认关闭 auto-commit，成功处理后会显式提交 offset；如需自动提交，请设置 `Consumer.AutoCommit=true`。

#### 直接使用

```go
import (
    "context"
    "github.com/aisgo/ais-pkg/mq"
    _ "github.com/aisgo/ais-pkg/mq/kafka"     // 注册 Kafka 实现
    _ "github.com/aisgo/ais-pkg/mq/rocketmq"  // 注册 RocketMQ 实现
    "go.uber.org/zap"
)

// 配置驱动 - 自动选择实现
cfg := &mq.Config{
    Type: mq.TypeKafka,
    Kafka: &mq.KafkaConfig{
        Brokers: []string{"localhost:9092"},
    },
}

producer, _ := mq.NewProducer(cfg, zap.NewNop())

// 发送消息
msg := mq.NewMessage("order-events", []byte(`{"order_id": 123}`)).
    WithKey("order-123").
    WithProperty("trace-id", "abc123")
_, _ = producer.SendSync(context.Background(), msg)

// 消费消息
consumer, _ := mq.NewConsumer(cfg, zap.NewNop())
_ = consumer.Subscribe("order-events", func(ctx context.Context, msgs []*mq.ConsumedMessage) (mq.ConsumeResult, error) {
    // TODO: 处理 msgs
    return mq.ConsumeSuccess, nil
})
_ = consumer.Start()
```

#### 使用 Fx 模块

```go
import (
    "context"
    "github.com/aisgo/ais-pkg/logger"
    "github.com/aisgo/ais-pkg/mq"
    "github.com/aisgo/ais-pkg/mq/rocketmq"
    "go.uber.org/fx"
)

app := fx.New(
    fx.Provide(
        func() logger.Config { return logger.Config{Level: "info"} },
        func() *rocketmq.Config {
            return &rocketmq.Config{
                NameServers: []string{"localhost:9876"},
                Producer: rocketmq.ProducerConfig{GroupName: "my-producer"},
                Consumer: rocketmq.ConsumerConfig{GroupName: "my-consumer"},
            }
        },
    ),
    logger.Module,
    rocketmq.Module,         // 严格模式：配置缺失或初始化失败时返回 error
    // 或使用 rocketmq.OptionalModule  // 宽松模式：配置缺失时返回 nil，不阻塞启动
    fx.Invoke(func(producer *rocketmq.Producer, consumer *rocketmq.Consumer) {
        // Producer 和 Consumer 会自动注入
    }),
)
```

> ✅ **Optional 模式**：使用 `rocketmq.OptionalModule` 或 `rocketmq.OptionalProvideProducer/OptionalProvideConsumer` 可实现配置缺失时返回 nil，不阻塞应用启动。适用于 MQ 作为可选依赖的场景。

### 🔐 Idempotency - 幂等性检查

基于 Redis 的幂等性检查器，防止消息/请求重复处理。

#### 直接使用

```go
import "github.com/aisgo/ais-pkg/idempotency"

// 创建幂等性检查器
checker := idempotency.New(redisClient, idempotency.Config{
    KeyPrefix:  "myapp:processed:",
    TTL:        24 * time.Hour,
    EnvModeKey: "MY_IDEMPOTENCY_MODE",  // 可选，默认 IDEMPOTENCY_MODE
})

// 方式一：先检查后标记（两步）
if processed, _ := checker.Check(ctx, "tenant:event123"); processed {
    return // 已处理
}
// ... 处理逻辑 ...
checker.Mark(ctx, "tenant:event123")

// 方式二：原子检查并标记（推荐，使用 SetNX）
if exists, _ := checker.CheckAndMark(ctx, "tenant:event123"); exists {
    return // 已处理
}
// ... 处理逻辑 ...
```

**运行模式（通过环境变量配置）：**
- `required`: Redis 不可用时返回错误，适合严格幂等场景
- `best_effort`: Redis 不可用时降级，适合可容忍少量重复的场景（默认）
- `disabled`: 跳过检查，适合测试或业务本身幂等的场景

#### 在 Worker 中使用

```go
import (
    "github.com/aisgo/ais-pkg/idempotency"
    "github.com/apache/rocketmq-client-go/v2/consumer"
    "github.com/apache/rocketmq-client-go/v2/primitive"
)

type MyWorker struct {
    idempotency *idempotency.Checker
    // ... other fields
}

func (w *MyWorker) HandleMessage(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
    for _, msg := range msgs {
        // 1. 解析消息
        event := parseEvent(msg.Body)
        
        // 2. 幂等检查
        key := fmt.Sprintf("%s:%s", event.TenantID, event.EventID)
        if processed, _ := w.idempotency.Check(ctx, key); processed {
            continue // 跳过已处理
        }
        
        // 3. 业务处理
        if err := w.process(ctx, event); err != nil {
            return consumer.ConsumeRetryLater, err
        }
        
        // 4. 标记已处理
        w.idempotency.Mark(ctx, key)
    }
    return consumer.ConsumeSuccess, nil
}
```

> ✅ **配置驱动**：运行模式可通过环境变量配置，便于不同环境切换
>
> ✅ **优雅降级**：Redis 不可用时可降级运行，保证系统可用性
>
> ✅ **原子操作**：`CheckAndMark` 使用 SetNX 实现原子检查并标记

### 🌐 Transport - HTTP/gRPC 服务器

#### HTTP Server (Fiber v3)

```go
import (
    aishttp "github.com/aisgo/ais-pkg/transport/http"
    "github.com/aisgo/ais-pkg/middleware"
    "github.com/aisgo/ais-pkg/logger"
    "github.com/gofiber/fiber/v3"
    "go.uber.org/fx"
)

app := fx.New(
    fx.Provide(
        logger.NewNop,
        func() aishttp.Config { 
            return aishttp.Config{
                Port: 8080,
                // CORS 配置（可选）
                CORS: middleware.CORSConfig{
                    Enabled: true,
                    AllowOrigins: []string{"https://example.com"},
                    AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
                    AllowCredentials: true,
                    MaxAge: 3600,
                },
            }
        },
        aishttp.NewHTTPServer,
    ),
    fx.Invoke(func(fiberApp *fiber.App) {
        fiberApp.Get("/api/health", func(c fiber.Ctx) error {
            return c.JSON(fiber.Map{"status": "ok"})
        })
    }),
)
_ = app
```

> ✅ CORS 中间件会在 Recover 中间件之后、业务路由之前自动注册。也可通过依赖注入 `*middleware.CORSConfig` 动态配置。

#### gRPC Server

```go
import (
    aisgrpc "github.com/aisgo/ais-pkg/transport/grpc"
    "github.com/aisgo/ais-pkg/logger"
    "go.uber.org/fx"
    "google.golang.org/grpc"
)

app := fx.New(
    fx.Provide(
        logger.NewNop,
        func() aisgrpc.Config { return aisgrpc.Config{Port: 50051, Mode: "microservice"} },
        aisgrpc.NewInProcListener,
        aisgrpc.NewListener,
        aisgrpc.NewServer,
    ),
    fx.Invoke(func(s *grpc.Server) {
        // 注册服务
        // pb.RegisterYourServiceServer(s, &yourService{})
    }),
)
_ = app
```

> ✅ gRPC ClientFactory 支持 TLS：配置 `aisgrpc.Config.TLS`（例如 `enable/ca_file/cert_file/key_file/server_name`）即可启用安全连接。

### 📊 Metrics - Prometheus 监控

#### 直接使用

```go
import "github.com/aisgo/ais-pkg/metrics"

// 注册指标
requestCounter := metrics.NewCounter("http_requests_total", "Total HTTP requests")
requestDuration := metrics.NewHistogram("http_request_duration_seconds", "HTTP request latency")

// 使用
requestCounter.Inc()
requestDuration.Observe(0.05)
```

#### 使用 Fx 模块

```go
import (
    "github.com/aisgo/ais-pkg/metrics"
    "go.uber.org/fx"
)

app := fx.New(
    metrics.Module,
    fx.Invoke(func() {
        // 注册指标
        requestCounter := metrics.NewCounter("http_requests_total", "Total HTTP requests")
        requestCounter.Inc()
    }),
)
```

### 🗂️ Repository - 数据仓储模式

提供通用 CRUD、分页、聚合等数据访问模式。

```go
import "github.com/aisgo/ais-pkg/repository"

type UserRepository struct {
    repository.Repository[User]
}

// 推荐使用 NewRepository 构造函数
repo := &UserRepository{
    Repository: repository.NewRepository[User](db),
}

// CRUD 操作
user := &User{Name: "Alice"}
_ = repo.Create(ctx, user)

// 分页查询
page, _ := repo.FindPageByModel(ctx, 1, 10, &User{Name: "Alice"})
```

#### 多租户 (默认强制)

Repository 默认强制租户隔离，请在调用前将租户信息注入 context。

```go
ctx := repository.WithTenantContext(ctx, repository.TenantContext{
    TenantID: tenantID,
    DeptID:   deptID,
    IsAdmin:  false,
})

err := repo.Create(ctx, user)
```

如需对非多租户表关闭强制隔离，实现接口即可：

```go
type NonTenantModel struct {
    ID   string `gorm:"column:id;type:char(26);primaryKey"`
    Name string `gorm:"column:name"`
}

func (NonTenantModel) TenantIgnored() bool { return true }
```

#### 聚合返回值说明

`Max/Min/MaxWithCondition/MinWithCondition` 的返回值类型由数据库驱动决定（如 `int64/float64/string/[]byte/time.Time` 等），
无记录时返回 `nil`。调用方应按实际类型进行断言或转换。

### ✅ Validator - 数据验证

基于 validator/v10 的验证器封装。

#### 直接使用

```go
import "github.com/aisgo/ais-pkg/validator"

type CreateUserRequest struct {
    Email    string `validate:"required,email"`
    Age      int    `validate:"gte=0,lte=120"`
    Username string `validate:"required,min=3,max=20"`
}

v := validator.New()
req := &CreateUserRequest{Email: "invalid", Age: 200}

if err := v.Validate(req); err != nil {
    // 处理验证错误
}
```

#### 使用 Fx 模块

```go
import (
    "github.com/aisgo/ais-pkg/validator"
    "go.uber.org/fx"
)

app := fx.New(
    validator.Module,
    fx.Invoke(func(v *validator.Validator) {
        // 使用验证器...
    }),
)
```

### 🆔 ULID - 分布式唯一 ID

基于 oklog/ulid/v2 的高性能 ID 生成器，支持时间排序、数据库友好。

#### 特性

- **128 位唯一性** - 48 位时间戳 + 80 位随机数
- **字典序排序** - 按时间戳自然排序，适合数据库索引
- **URL 安全** - Crockford's Base32 编码，26 字符
- **数据库友好** - `ulid.ID` 类型支持 PostgreSQL `bytea` 存储
- **JSON 友好** - 序列化为可读字符串，非 base64

#### 基本使用

```go
import "github.com/aisgo/ais-pkg/ulid"

// 生成 ULID
id := ulid.Generate()
fmt.Println(id.String())  // 01HN3K8X9FQZM6Y8VWXQR2JNPT

// 生成字符串格式
str := ulid.GenerateString()

// 解析 ULID
parsed, err := ulid.Parse("01HN3K8X9FQZM6Y8VWXQR2JNPT")

// 提取时间戳
timestamp := ulid.Time(id)

// 批量生成（同一毫秒内递增）
ids := ulid.GenerateBatch(100)
```

#### 数据库模型使用 (ulid.ID 类型)

`ulid.ID` 是为数据库和 JSON 序列化优化的包装类型：

```go
import "github.com/aisgo/ais-pkg/ulid"

type User struct {
    ID        ulid.ID   `json:"id" gorm:"type:bytea;primaryKey"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
}

// 生成新 ID
user := &User{
    ID:   ulid.NewID(),
    Name: "Alice",
}

// 解析 ID
id, err := ulid.ParseID("01HN3K8X9FQZM6Y8VWXQR2JNPT")

// 零值检查
if id.IsZero() {
    // 处理空 ID
}
```

#### 存储格式

| 场景 | 格式 | 大小 |
|------|------|------|
| PostgreSQL | `bytea` | 16 bytes |
| JSON | `"01HN3K8X9FQZM6Y8VWXQR2JNPT"` | 26 字符字符串 |
| 零值 (DB) | `NULL` | - |
| 零值 (JSON) | `null` | - |

#### 零值处理

所有序列化格式对零值的处理语义一致：

```go
var zero ulid.ID

// 数据库: NULL
value, _ := zero.Value()  // nil

// JSON: null
data, _ := json.Marshal(zero)  // "null"

// Text/Binary: 空
text, _ := zero.MarshalText()  // []byte{}
```

#### 输入容错

反序列化方法会自动裁剪空白，兼容表单/数据库输入：

```go
// 以下输入都会正确解析或返回零值
id.UnmarshalText([]byte("  01HN3K8X9F...  \n"))  // 正确解析
id.UnmarshalText([]byte("   "))                   // 零值
id.Scan("  01HN3K8X9F...  ")                      // 正确解析
```

#### UUID 互转

```go
// ULID -> UUID
uuidVal := ulid.ToUUID(id)
uuidStr := ulid.ToUUIDString(id)

// UUID -> ULID
ulidVal := ulid.FromUUID(uuidVal)
ulidVal, err := ulid.FromUUIDString("550e8400-e29b-41d4-a716-446655440000")
```

### 🛑 Shutdown - 优雅关闭

分优先级管理资源清理顺序。

#### 直接使用

```go
import "github.com/aisgo/ais-pkg/shutdown"

manager := shutdown.NewManager(log)

// 注册清理函数（优先级：数字越小越先执行）
manager.Register(shutdown.PriorityHigh, func(ctx context.Context) error {
    return httpServer.Shutdown(ctx)
})

manager.Register(shutdown.PriorityMedium, func(ctx context.Context) error {
    return db.Close()
})

// 等待信号并执行清理
manager.Wait()
```

#### 使用 Fx 模块

```go
import (
    "github.com/aisgo/ais-pkg/shutdown"
    "go.uber.org/fx"
)

app := fx.New(
    shutdown.Module,
    fx.Invoke(func(manager *shutdown.Manager) {
        // 注册清理函数
        manager.Register(shutdown.PriorityHigh, func(ctx context.Context) error {
            return httpServer.Shutdown(ctx)
        })
    }),
)
```

---

## 🏗️ 架构设计

### 设计原则

1. **接口优先** - 所有组件基于接口设计，便于 Mock 和替换
2. **配置驱动** - 通过配置文件和环境变量控制行为
3. **依赖注入** - 支持 Uber Fx，也可独立使用
4. **错误透明** - 统一错误处理和转换机制
5. **可观测性** - 内置日志、指标、链路追踪支持

### 目录结构

```
ais-pkg/
├── cache/              # 缓存组件
│   └── redis/          # Redis 实现（支持 Optional 模式）
├── conf/               # 配置加载
├── database/           # 数据库连接
│   └── postgres/       # PostgreSQL 实现
├── errors/             # 错误定义
├── logger/             # 日志组件
├── metrics/            # 监控指标
├── middleware/         # HTTP 中间件
├── mq/                 # 消息队列
│   ├── kafka/          # Kafka 适配器
│   └── rocketmq/       # RocketMQ 适配器（支持 Optional 模式）
├── idempotency/        # 幂等性检查
│   └── checker.go      # Redis 幂等性检查器
├── repository/         # 数据仓储
├── response/           # 响应封装
├── shutdown/           # 优雅关闭
├── transport/          # 传输层
│   ├── http/           # HTTP 服务器
│   └── grpc/           # gRPC 服务器
├── ulid/               # ULID 生成器
│   ├── ulid.go         # 核心生成器
│   ├── id.go           # 数据库友好 ID 类型
│   └── convert.go      # UUID 互转
├── utils/              # 工具函数
└── validator/          # 数据验证
```

---

## 🔧 开发指南

### 添加新组件

1. 在对应目录创建包
2. 定义清晰的接口
3. 提供配置结构体
4. 实现 Fx 模块（可选）
5. 编写单元测试
6. 更新文档

### 测试

```bash
# 运行所有测试
go test ./...

# 带覆盖率
go test -cover ./...

# 指定包
go test ./logger -v
```

### 代码规范

- 遵循 [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- 所有公共 API 必须有注释
- 使用 ASCII 风格分块注释组织代码
- 错误处理必须明确，不吞噬错误

---

## 📋 依赖清单

### 核心依赖

| 库 | 版本 | 用途 |
|----|------|------|
| go.uber.org/zap | v1.27.1 | 结构化日志 |
| go.uber.org/fx | v1.24.0 | 依赖注入 |
| github.com/spf13/viper | v1.21.0 | 配置管理 |
| gorm.io/gorm | v1.31.1 | ORM 框架 |
| github.com/redis/go-redis/v9 | v9.17.2 | Redis 客户端 |
| github.com/gofiber/fiber/v3 | v3.0.0-rc.3 | HTTP 框架 |
| google.golang.org/grpc | v1.78.0 | gRPC 框架 |
| github.com/IBM/sarama | v1.46.3 | Kafka 客户端 |
| github.com/apache/rocketmq-client-go/v2 | v2.1.2 | RocketMQ 客户端 |
| github.com/prometheus/client_golang | v1.23.2 | Prometheus 客户端 |
| github.com/go-playground/validator/v10 | v10.30.1 | 数据验证 |
| github.com/oklog/ulid/v2 | v2.1.0 | ULID 生成 |

---

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

### 提交规范

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Type:**
- `feat`: 新功能
- `fix`: 修复 Bug
- `docs`: 文档更新
- `refactor`: 代码重构
- `test`: 测试相关
- `chore`: 构建/工具链

---

## 📄 License

MIT License - 详见 [LICENSE](LICENSE) 文件

---

## 🔗 相关资源

- [CLAUDE.md](CLAUDE.md) - 详细架构文档
- [Go 官方文档](https://go.dev/doc/)
- [Uber Go Style Guide](https://github.com/uber-go/guide)

---

<div align="center">
Made with ❤️ by AIS Team
</div>

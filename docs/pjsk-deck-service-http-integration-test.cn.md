# PJSK Deck-Service HTTP 联调测试记录

截至 2026-03-31，这轮 `deck-service <-> Haruki-Cloud` 联调已经完成了 Cloud 侧 HTTP 接线、兼容层验证和真实服务调用验证。

这份文档用于记录：

- 本轮联调的背景和输入数据；
- `Haruki-Cloud` 为接入 `deck-service` 做了哪些改动；
- 实际测试命令和测试结果；
- 当前已经确认的结论；
- 尚未解决但已经定位清楚的剩余问题。

## 1. 背景

`deck-service` 是从 `Haruki-Cloud` 中原本的本地 deck native 能力拆出来的独立 HTTP 服务。

旧路径为：

- `internal/pjsk/render/deck/deck_cgo`

本轮目标是：

- 启动独立的 `deck-service`；
- 让 `Haruki-Cloud` 的 `deck recommend auto` 优先改走 HTTP 服务；
- 通过 `api/legacy/pjsk` 兼容层验证，而不是走客户端；
- 用户数据使用 `Haruki-Cloud/Data/collections.suite.json`；
- masterdata 使用 `deckrec/masterdata`；
- 画图仍然沿用远程 drawing 服务。

## 2. 本轮测试输入

本轮实际使用的输入如下：

- 用户快照：`Haruki-Cloud/Data/collections.suite.json`
- masterdata 根目录：`deckrec/masterdata`
- 音游 meta 文件：测试时使用 `/tmp/music_metas-jp.json`
- Cloud 入口：`api/legacy/pjsk`
- drawing：继续走远程 drawing endpoint

补充说明：

- `collections.suite.json` 不是传统的纯 `suite.json`，而是 Mongo Extended JSON 的单文档数组包装格式。
- 因此，Cloud 侧必须先做标准化，不能直接原样交给原有 `userdata` 解码路径。

## 3. deck-service 二进制选择

用户提供了两个可执行文件：

- `deck-service/build/deck-service`
- `deck-service/build/deck-service (1)`

当前测试环境是 Linux。

实际验证结果：

- `deck-service/build/deck-service (1)` 在当前环境执行会报 `Exec format error`
- 该文件包含 `/usr/lib/dyld`、`/usr/lib/libSystem.B.dylib`、`__PAGEZERO` 等 Mach-O 特征
- 因此它不是当前 Linux 环境可执行的二进制

所以，本轮联调实际使用的是：

- `deck-service/build/deck-service`

## 4. Haruki-Cloud 侧改动

### 4.1 配置新增 HTTP deck-service 入口

`deck_recommend` 配置新增：

- `service_base_url`

接线位置包括：

- `config/config.go`
- `cmd/server/main.go`
- `internal/pjsk/render/app/app.go`
- `haruki-db-configs.example.yaml`

当前行为是：

- 若 `deck_recommend.service_base_url` 非空，则优先走远程 HTTP deck-service
- 否则若 `use_local_engine` 为真，则走本地 cgo engine
- 否则回退到原本的本地启发式 fallback

### 4.2 deck controller 新增远程 engine provider

`internal/pjsk/render/deck/remote_engine.go` 新增了远程推荐实现。

当前远程推荐流程为：

1. 按区服初始化远端 recommender。
2. 首次调用前向 `deck-service` 发送 `/update/masterdata`。
3. 首次调用前向 `deck-service` 发送 `/update/musicmetas`。
4. 推荐阶段调用 `/recommend`。

### 4.3 改为优先传本地文件路径，避免大请求体

`collections.suite.json` 体积约 9 MB。

为避免 deck-service 在接收大 JSON 字符串时出现 body 限制或额外传输成本，Cloud 侧改成优先传文件路径：

- 用户数据优先传 `user_data_file_path`
- `music_metas` 优先通过 `/update/musicmetas` 的 `file_path` 方式加载

这样 deck-service 在推荐请求中不再需要接收超大 `user_data_str`。

### 4.4 本地快照标准化

`internal/pjsk/render/userdata` 新增了标准化能力：

- 顶层单元素数组解包
- `$numberLong`
- `$numberInt`
- `$numberDouble`
- `$numberDecimal`
- `$oid`
- `$date`

标准化后：

- 既保留内存中的 `RawUserData`
- 也会写出一个标准化后的临时 JSON 文件供 deck-service 直接读取

### 4.5 deck 请求默认参数做了收口

为避免 deck-service 被错误默认值影响，本轮还修了两类行为：

- 非 `multi` live 时不再默认注入 `multi_live_teammate_*`
- 不再默认发送空的 `fixed_cards` / `fixed_characters`

这两点都属于“Cloud 端默认请求清理”，避免服务端把“字段存在”误判为“显式约束”。

## 5. 实际启动方式

本轮实际启动命令为：

```bash
cd /home/xmlq/codes/haruki/deck-service
BIND_ADDR=127.0.0.1:48080 \
DECK_DATA_DIR=/home/xmlq/codes/haruki/deckrec/masterdata \
./build/deck-service
```

说明：

- 使用 `48080` 只是本轮联调时选定的可用端口
- `DECK_DATA_DIR` 指向 `deckrec/masterdata`

## 6. 兼容层与集成测试

### 6.1 兼容层路由验证

实际通过的测试包括：

```bash
go test ./api/legacy/pjsk -run 'TestPJSKDeckRecommendAutoBuildRouteReturnsBuiltPayload|TestPJSKDeckRecommendAutoRenderRouteReturnsDrawingBytes' -v
```

结果：

- `TestPJSKDeckRecommendAutoBuildRouteReturnsBuiltPayload` 通过
- `TestPJSKDeckRecommendAutoRenderRouteReturnsDrawingBytes` 通过

这说明：

- `api/legacy/pjsk` 兼容层没有被这轮接线改坏
- `deck recommend auto` 的 build/render 兼容入口仍可正常工作

### 6.2 远程 deck-service 集成测试

新增了真实联调测试，用于验证 Cloud 是否真的能打到 deck-service：

```bash
HARUKI_DECK_SERVICE_URL=http://127.0.0.1:48080 \
HARUKI_TEST_USER_JSON=/home/xmlq/codes/haruki/Haruki-Cloud/Data/collections.suite.json \
HARUKI_DECK_MASTERDATA_DIR=/home/xmlq/codes/haruki/deckrec/masterdata \
HARUKI_TEST_MUSIC_META_JSON=/tmp/music_metas-jp.json \
go test ./internal/pjsk/render/deck -run TestBuildAutoRecommendRequestWithDeckServiceIntegration -v
```

测试期间确认到的行为：

- Cloud 能成功调用 `/update/masterdata`
- Cloud 能成功调用 `/update/musicmetas`
- Cloud 能成功调用 `/recommend`
- deck-service 实际读到了用户卡牌数据，并在报错中返回 `862 cards`

也就是说：

- HTTP 链路是通的
- masterdata 加载是通的
- 用户快照标准化与文件路径传递是通的
- 失败点已经进入 deck 引擎业务层，而不是网络层或协议层

### 6.3 本地 cgo engine 对照测试

为了排除“HTTP 改造引入回归”，额外新增了一个本地 cgo 对照测试：

```bash
HARUKI_TEST_ENABLE_LOCAL_ENGINE=1 \
HARUKI_TEST_USER_JSON=/home/xmlq/codes/haruki/Haruki-Cloud/Data/collections.suite.json \
HARUKI_DECK_MASTERDATA_DIR=/home/xmlq/codes/haruki/deckrec/masterdata \
HARUKI_TEST_MUSIC_META_JSON=/tmp/music_metas-jp.json \
HARUKI_TEST_ALGORITHM=ga \
go test -tags pjsk_deck_cgo ./internal/pjsk/render/deck -run TestBuildAutoRecommendRequestWithLocalEngineIntegration -v
```

结果：

- 本地 cgo engine 也返回 `Cannot recommend any deck in 862 cards`

这一步非常关键，因为它说明：

- 同一份用户数据
- 同一份 masterdata
- 同一组测试参数
- 无论走 HTTP deck-service 还是本地 cgo engine

最终都得到同样的业务失败结果。

因此，本轮已经可以排除：

- Cloud HTTP 接线错误
- 远程请求字段名错误
- 快照标准化导致的数据结构破坏
- 仅远程服务路径才存在的行为回归

## 7. 已通过的相关测试

本轮确认通过的测试包括：

```bash
go test ./internal/pjsk/render/deck
go test ./internal/pjsk/render/userdata
go test ./cmd/server
go test ./api/legacy/pjsk -run 'TestPJSKDeckRecommendAutoBuildRouteReturnsBuiltPayload|TestPJSKDeckRecommendAutoRenderRouteReturnsDrawingBytes' -v
```

补充说明：

- `./api/legacy/pjsk` 仍存在一个与本轮任务无关的其他测试失败项
- 本轮 deck-service 联调不依赖那个失败项

## 8. 当前结论

截至当前，可以确认以下结论：

1. `Haruki-Cloud` 已经支持通过 `deck_recommend.service_base_url` 切换到远程 deck-service。
2. `api/legacy/pjsk` 兼容层可以继续使用，不需要通过客户端链路做本轮验证。
3. `collections.suite.json` 已经可以被 Cloud 标准化并安全交给 deck-service 使用。
4. 远程 deck-service 的 masterdata 和 music meta 初始化链路已经验证通过。
5. 当前剩余问题不是“联不通”，而是“这组数据和参数在底层引擎里推不出可用 deck”。

## 9. 当前未解决的问题

当前真实推荐仍然失败，典型错误为：

```text
Cannot recommend any deck in 862 cards
```

这说明问题已经收敛到以下范围之一：

- 当前测试使用的 `music_metas` 数据过于简化
- 当前测试参数组合不适合这份用户数据
- 底层推荐引擎本身对该组合存在更严格的业务前提

## 10. 后续建议

若要把状态推进到“真实返回至少一条 deck 结果”，建议按下面顺序继续排查：

1. 换成真实且完整的 `music_metas.json`，不要只用测试用最小样本。
2. 扫描更贴近真实使用场景的参数组合，尤其是：
   - `recommend_type`
   - `target`
   - `live_type`
   - `algorithm`
3. 若仍失败，再进一步把 Cloud 发给 deck-service 的完整 option payload 打印出来，与历史可用请求做对照。

## 11. 涉及的主要文件

本轮相关代码和测试主要落在以下位置：

- `config/config.go`
- `cmd/server/main.go`
- `internal/pjsk/render/app/app.go`
- `internal/pjsk/render/deck/controller.go`
- `internal/pjsk/render/deck/recommender.go`
- `internal/pjsk/render/deck/remote_engine.go`
- `internal/pjsk/render/deck/integration_test.go`
- `internal/pjsk/render/userdata/local.go`
- `internal/pjsk/render/userdata/normalize.go`
- `haruki-db-configs.example.yaml`

## 12. 一句话总结

本轮已经完成“Cloud 接入远程 deck-service 并成功把请求打到引擎”的目标；当前没有证据表明 HTTP 化改坏了推荐逻辑，剩余问题集中在底层推荐业务结果本身。

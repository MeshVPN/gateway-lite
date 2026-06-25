# OpenP2P 自建网关 · 使用与启动测试文档

本文档介绍如何编译、启动并测试本仓库中的 **自建信令网关（gateway-lite）** 与 **P2P 客户端（openp2p）**，并说明新增的 Web 管理控制台、虚拟网络（SDWAN）、多用户注册、设备日志查看与登录安全加固等能力。

> 适用范围：本仓库经过定制，使开源服务端 `gateway-lite` 具备了官方控制台（console.openp2p.cn）的核心管理能力，可完全自托管，无需依赖官方服务器。

---

## 一、项目结构

```
openp2p/
├── gateway-lite/      # 自建信令网关（服务端，本文重点）
│   ├── webui/         # 内嵌 Web 管理控制台（单页 HTML）
│   ├── docker/        # 服务端 Dockerfile
│   ├── api.crt/api.key# 自签名 TLS 证书
│   ├── store.go       # JSON 文件持久化（用户/设备/虚拟网络）
│   ├── webmgmt.go     # 注册/profile/虚拟网络管理 REST 接口
│   ├── webui.go       # go:embed 嵌入前端页面
│   ├── loginsecurity.go # 弱口令校验 + 防暴力破解
│   └── ...
└── openp2p/           # P2P 客户端
    ├── cmd/openp2p.go # 客户端入口
    └── core/          # 客户端核心逻辑
```

---

## 二、环境要求

| 组件 | Go 版本要求 | 说明 |
| --- | --- | --- |
| **服务端 gateway-lite** | Go 1.18+（兼容最新版） | 无特殊依赖约束，可直接用本机 Go 编译 |
| **客户端 openp2p** | **必须 Go 1.20.x** | 依赖 `quic-go v0.34.0`，**无法在 Go 1.21+ 编译**（编译会报 “can't be built on Go 1.21 yet”） |

> ⚠️ **关键约束**：若本机 Go 版本高于 1.20（如 1.21/1.22/1.25），编译客户端时必须通过 `GOTOOLCHAIN=go1.20.14` 指定工具链，Go 会自动下载并使用 1.20 编译。服务端不受此限制。

统一建议设置代理：
```bash
export GOPROXY=https://goproxy.io,direct
```

---

## 三、服务端：编译与启动

### 3.1 编译

```bash
cd gateway-lite
export GOPROXY=https://goproxy.io,direct
go mod tidy
go build -o openp2p-gateway .
```

### 3.2 启动

服务端通过环境变量配置内置管理员账号（用户名长度需 ≥ 8）：

```bash
export OPENP2P_USER=youradmin          # 管理员用户名（≥8 字符）
export OPENP2P_PASSWORD='YourStr0ng#Pwd' # 管理员密码
./openp2p-gateway
```

启动成功后日志示例：
```
INFO TOKEN:12736910170512452892
INFO store loaded: 1 users, 0 devices, 0 networks
[GIN-debug] Listening and serving HTTPS on :10008
[GIN-debug] Listening and serving HTTPS on :27183
```

### 3.3 Docker 启动（可选）

```bash
docker run -d --restart always --net=host \
  -e OPENP2P_USER=youradmin \
  -e OPENP2P_PASSWORD='YourStr0ng#Pwd' \
  --name openp2p-gateway openp2pcn/openp2p-gateway:latest
```

### 3.4 端口与防火墙

| 协议 | 端口 | 用途 |
| --- | ---: | --- |
| TCP | 10008 | HTTPS REST API + Web 控制台 |
| TCP | 27180 | STUN / ifconfig（NAT 探测） |
| TCP | 27181 | STUN / ifconfig（NAT 探测） |
| TCP | 27183 | 客户端 WebSocket 信令 |
| UDP | 27182 | STUN UDP（NAT 类型探测） |
| UDP | 27183 | STUN UDP（NAT 类型探测） |

> 端口 6060 为 Go pprof 调试端口，仅监听本机，生产环境可忽略。

---

## 四、Web 管理控制台

服务端内置了一个单页管理控制台（通过 `go:embed` 打包进二进制，无需额外部署）。

浏览器访问：
```
https://<网关IP或域名>:10008/
```

> 由于使用自签名证书，浏览器会提示“不安全”，点击继续访问即可。

控制台包含四个页面，对应官方 console.openp2p.cn：

| 页面 | 功能 |
| --- | --- |
| **设备** | 在线/离线设备列表、改名、重启、**查看实时日志** |
| **虚拟网络** | 创建/编辑/删除虚拟网络，添加成员并自动分配虚拟 IP，启用/禁用 |
| **安装** | 展示网关地址、登录 Token 与客户端启动命令示例 |
| **我** | 个人信息（用户名/Token/注册时间），编辑邮箱、手机 |

控制台支持**注册新账号**（多用户），每个用户的数据相互隔离。

---

## 五、客户端：编译与连接

### 5.1 编译（注意 Go 版本）

```bash
cd openp2p
export GOPROXY=https://goproxy.io,direct

# 若本机 Go 为 1.20.x：
go build -o openp2p-client ./cmd/openp2p.go

# 若本机 Go 高于 1.20（如 1.21+/1.25），必须指定工具链：
GOTOOLCHAIN=go1.20.14 go build -o openp2p-client ./cmd/openp2p.go
```

或使用官方预编译二进制（[releases](https://github.com/openp2p-cn/openp2p/releases)）。

### 5.2 连接到自建网关

客户端通过以下参数连接，其中 `token` 为登录接口返回的 `nodeToken`：

```bash
./openp2p-client \
  -serverhost <网关IP或域名> \
  -serverport 27183 \
  -node mypc-node-01 \
  -token <nodeToken> \
  -insecure
```

关键参数说明：

| 参数 | 说明 |
| --- | --- |
| `-serverhost` | 自建网关的 IP 或域名 |
| `-serverport` | 信令端口，自建网关为 **27183** |
| `-node` | 本节点名称，8–31 字符；不填则用主机名 |
| `-token` | 登录返回的 `nodeToken`（即 `crc64(user+password)`） |
| `-insecure` | **自签名证书必须加此参数**，否则 TLS 校验失败无法连接 |
| `-d` | 以守护进程模式运行 |
| `-sharebandwidth` | 共享带宽限制（Mbps），默认 10 |

> 中继节点：当两个节点无法直接打洞时需要中继。建议在服务器上也安装一个客户端作为中继节点（参数同上，`-node` 取一个独立名称）。

---

## 六、REST API 速查

所有接口基于 `https://<网关>:10008`，自签名证书需 `curl --insecure`（下文简写 `-k`）。

### 6.1 登录（获取 JWT 与 nodeToken）

```bash
curl -k "https://<网关>:10008/api/v1/user/login" -X POST \
  -H "Content-Type: application/json" \
  -d '{"user":"youradmin","password":"YourStr0ng#Pwd"}'
```
返回：
```json
{"error":0,"nodeToken":"12736910170512452892","token":"<JWT>"}
```
- `nodeToken`：用于客户端 `-token` 连接
- `token`（JWT）：用于后续管理 API 的 `Authorization` 请求头

### 6.2 注册新用户（含弱口令校验）

```bash
curl -k "https://<网关>:10008/api/v1/user/register" -X POST \
  -H "Content-Type: application/json" \
  -d '{"user":"newuser01","password":"Str0ng#Pwd"}'
```

### 6.3 个人信息

```bash
# 查询
curl -k "https://<网关>:10008/api/v1/user/profile" -H "Authorization: <JWT>"
# 更新邮箱/手机
curl -k "https://<网关>:10008/api/v1/user/profile" -X POST \
  -H "Authorization: <JWT>" -H "Content-Type: application/json" \
  -d '{"email":"a@b.com","phone":"13800138000"}'
```

### 6.4 设备管理

```bash
# 设备列表
curl -k "https://<网关>:10008/api/v1/devices" -H "Authorization: <JWT>"
# 重启在线设备
curl -k "https://<网关>:10008/api/v1/device/<node>/restart" -H "Authorization: <JWT>"
# 改名
curl -k "https://<网关>:10008/api/v1/device/<node>/editnode" -X POST \
  -H "Authorization: <JWT>" -d '{"newName":"newnodename"}'
# 查看实时日志（offset 增量拉取）
curl -k "https://<网关>:10008/api/v1/device/<node>/log?offset=0&len=65536" \
  -H "Authorization: <JWT>"
```

### 6.5 虚拟网络（SDWAN）

```bash
# 列表
curl -k "https://<网关>:10008/api/v1/sdwan/networks" -H "Authorization: <JWT>"
# 创建/编辑（带 id 为编辑）
curl -k "https://<网关>:10008/api/v1/sdwan/network" -X POST \
  -H "Authorization: <JWT>" -H "Content-Type: application/json" \
  -d '{"name":"mynet","gateway":"10.2.3.1/24","mode":"fullmesh","enable":1}'
# 添加成员（ip 留空自动分配）
curl -k "https://<网关>:10008/api/v1/sdwan/network/<id>/member" -X POST \
  -H "Authorization: <JWT>" -H "Content-Type: application/json" \
  -d '{"name":"node-alpha","enable":1}'
# 删除成员
curl -k "https://<网关>:10008/api/v1/sdwan/network/<id>/member/delete" -X POST \
  -H "Authorization: <JWT>" -d '{"name":"node-alpha"}'
# 删除网络
curl -k "https://<网关>:10008/api/v1/sdwan/network/<id>/delete" -X POST \
  -H "Authorization: <JWT>"
```

---

## 七、端到端启动测试流程

以下步骤验证从服务端启动到客户端互联的完整链路。

### 步骤 1：启动并自检服务端

```bash
cd gateway-lite
go build -o openp2p-gateway .
export OPENP2P_USER=youradmin OPENP2P_PASSWORD='YourStr0ng#Pwd'
./openp2p-gateway
```

新开终端，验证 Web 页面与登录：
```bash
# 页面可访问
curl -k -o /dev/null -w "page HTTP %{http_code}\n" https://127.0.0.1:10008/

# 登录拿到 JWT
TOKEN=$(curl -sk "https://127.0.0.1:10008/api/v1/user/login" -X POST \
  -H "Content-Type: application/json" \
  -d '{"user":"youradmin","password":"YourStr0ng#Pwd"}' | sed 's/.*"token":"//;s/".*//')

# 设备列表（初始为空）
curl -k "https://127.0.0.1:10008/api/v1/devices" -H "Authorization: $TOKEN"
```

### 步骤 2：连接两个客户端

在 PC1、PC2（或同机不同 node 名）分别运行：
```bash
./openp2p-client -serverhost 127.0.0.1 -serverport 27183 \
  -node mypc-node-01 -token <nodeToken> -insecure
```
连接成功后，刷新控制台“设备”页面应能看到在线设备。

### 步骤 3：建立 P2P 应用（端口转发）

将 PC1 的本地 23389 端口转发到 PC2 的 22 端口：
```bash
curl -k "https://127.0.0.1:10008/api/v1/device/mypc-node-01/app" -X POST \
  -H "Authorization: $TOKEN" -H "Content-Type: application/json" \
  -d '{"appName":"ssh","protocol":"tcp","srcPort":23389,"peerNode":"mypc-node-02","dstHost":"localhost","dstPort":22}'
```
随后访问 `localhost:23389` 即等于访问 PC2 的 22 端口。

### 步骤 4：验证设备日志

在控制台“设备”页点击在线设备的「查看日志」按钮，可看到该设备的实时运行日志（每 2 秒增量刷新）。

### 步骤 5：代码自检（开发者）

```bash
# 服务端
cd gateway-lite
go build ./... && go vet ./...
# 单元测试（排除依赖外网/主程序初始化的 TestNetInfo）
go test -run 'TestAESCBC|TestCompareVersion' .

# 客户端（注意工具链）
cd ../openp2p
GOTOOLCHAIN=go1.20.14 go build ./cmd/openp2p.go
GOTOOLCHAIN=go1.20.14 go vet ./core/
```

> 说明：`go test ./...` 中的 `TestNetInfo` 会请求外网 `ifconfig.co`，且 `gLog` 仅在 `main()` 中初始化，单测环境下为 `nil`，因此该用例在离线/测试环境会 panic。这是上游原有代码的测试脆弱性，与本次改动无关，故自检时按上面的命令跳过该用例。

---

## 八、安全特性说明

本网关在登录与注册环节做了如下加固（详见 `gateway-lite/loginsecurity.go`）：

- **弱口令拦截**（注册时）：密码长度 ≥ 8、不在常见弱口令黑名单、且至少包含大小写字母/数字/符号中的两类。
- **防暴力破解**：按「客户端 IP + 用户名」统计失败次数，10 分钟内连续失败 5 次后锁定 10 分钟；锁定期内即使密码正确也拒绝（返回 HTTP 429）。
- **时序攻击防护**：使用 `subtle.ConstantTimeCompare` 做密码比较，用户不存在时也执行比较以保持响应时间一致。

> 生产环境补充建议：
> - 当前密码以明文存储于 `gateway-data.json`，建议改为 bcrypt 哈希。
> - 限速为内存级，重启清零；如需持久化或多实例共享，可接入 Redis。
> - 建议替换自签名证书为受信任的正式证书，并去除客户端 `-insecure`。

---

## 九、数据持久化

- 服务端运行时数据保存在二进制同目录下的 **`gateway-data.json`**（用户、设备、虚拟网络），通过临时文件 + 原子重命名写入，文件权限 0600。
- 该文件已加入 `.gitignore`，不会被提交到仓库。

---

## 十、常见问题排查

| 现象 | 原因与解决 |
| --- | --- |
| 客户端编译报 “can't be built on Go 1.21 yet” | 本机 Go 版本过高，使用 `GOTOOLCHAIN=go1.20.14 go build ...` |
| 客户端连接失败 / TLS 证书错误 | 自签名证书需加 `-insecure` 参数 |
| 客户端连不上、设备列表为空 | 检查 27183(TCP/UDP)、27180/27181(TCP)、27182(UDP) 端口是否放通 |
| 登录返回 HTTP 429 | 触发防暴力破解锁定，等待提示的秒数后重试 |
| 注册返回 error:5 | 密码不符合强度要求，按提示调整 |
| 浏览器访问控制台提示不安全 | 自签名证书所致，点击“继续访问”即可 |
| 虚拟网络成员在客户端不生效 | 确认网络与成员的 `enable` 均为 1（仅启用的成员会下发给客户端） |

---

## 十一、检查结论（本次全量自检）

- ✅ **服务端 gateway-lite**：`go build ./...` / `go vet ./...` 全部通过，无错误；单元测试 `TestAESCBC`、`TestCompareVersion` 通过。
- ⚠️ **已知测试脆弱性**：`TestNetInfo` 会请求外网并依赖 `main()` 中才初始化的 `gLog`，离线/测试环境下会 panic。这是上游原有代码问题，与本次改动无关；自检请按第七章步骤 5 的命令跳过该用例。
- ✅ **客户端 openp2p**：源码无问题；在 `GOTOOLCHAIN=go1.20.14` 下 `go build` / `go vet` 通过。唯一约束是 `quic-go v0.34.0` 不支持 Go 1.21+，属依赖的预存限制，非代码缺陷。

# NextShell Cloud Sync Server

轻量自部署的云同步服务端，让同一用户在多台设备间共享连接、SSH 密钥和代理配置。

## 特性

- 单二进制，零外部依赖（内嵌 SQLite）
- 强制 HTTPS，Basic Auth 认证
- Workspace 自动创建，密码 bcrypt 加密存储
- 资源引用完整性检查（删除被引用的 SSH Key / Proxy 返回 409）
- 删除墓碑机制，确保跨设备同步删除
- 版本号单调递增，支持增量检测

## 快速开始

### 构建

```bash
cd server
go build -o nshellserver .
```

### 生成 TLS 证书

生产环境建议使用 Let's Encrypt 等 CA 签发的证书。本地测试可用自签名证书：

```bash
openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem \
  -days 365 -nodes -subj '/CN=localhost'
```

### 启动

```bash
./nshellserver --cert cert.pem --key key.pem
```

开发模式也可以直接运行：

```bash
./nshellserver --dev
```

`--dev` 会在当前运行目录查找 `nshell-key.pem` 和 `nshell-crt.pem`。如果两个文件都不存在，程序会直接调用系统 `openssl` 生成并复用这对自签名证书；如果只存在其中一个，则启动失败。`--dev` 不能与 `--cert` 或 `--key` 混用。

自托管并由外部反向代理终止 HTTPS 时，可以直接监听 HTTP：

```bash
./nshellserver --self-host
```

这会强制服务监听 `127.0.0.1`，不加载证书，适合只让本机反向代理转发。

如果确实需要直接对外暴露 HTTP 监听，可以显式运行：

```bash
./nshellserver --self-host --public
```

这会打印不安全警告，并把监听地址强制改为 `0.0.0.0`。`--public` 只能与 `--self-host` 一起使用。`--self-host` 不能与 `--cert`、`--key` 或 `--dev` 混用。

### 配置项

| 参数 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `--addr` | `NSHELL_ADDR` | `:8443` | 监听地址 |
| `--dev` | - | `false` | 开发模式：在当前目录生成并复用 `nshell-key.pem` / `nshell-crt.pem` |
| `--self-host` | - | `false` | 自托管 HTTP 模式：忽略证书并强制监听本机回环地址 |
| `--public` | - | `false` | 仅配合 `--self-host` 使用：改为监听 `0.0.0.0` 并打印不安全警告 |
| `--cert` | `NSHELL_CERT` | （必填） | TLS 证书文件 |
| `--key` | `NSHELL_KEY` | （必填） | TLS 私钥文件 |
| `--db` | `NSHELL_DB` | `./nshell.db` | SQLite 数据库路径 |

CLI 参数优先于环境变量。

### 验证

```bash
curl -k -X POST https://localhost:8443/api/v1/sync/workspace/status \
  -u "myworkspace:mypassword" \
  -H "Content-Type: application/json" \
  -d '{}'
```

成功返回：

```json
{"ok": true, "workspace": "myworkspace", "version": 0, "serverTime": "..."}
```

## 项目结构

```
server/
├── main.go                        # 入口：CLI、TLS server、路由、graceful shutdown
├── internal/
│   ├── config/config.go           # 配置加载
│   ├── db/
│   │   ├── db.go                  # SQLite 连接、pragma、迁移
│   │   └── store.go               # 数据层 CRUD
│   ├── handler/
│   │   ├── handler.go             # Handler + JSON 工具函数
│   │   ├── middleware.go          # Basic Auth + Body Limit
│   │   ├── workspace.go          # workspace/status
│   │   ├── pull.go               # pull
│   │   ├── connection.go         # connections upsert/delete
│   │   ├── sshkey.go             # ssh-keys upsert/delete
│   │   └── proxy.go              # proxies upsert/delete
│   └── model/model.go            # 请求/响应结构体
```

## 依赖

| 包 | 用途 |
|---|---|
| `github.com/go-chi/chi/v5` | HTTP 路由 |
| `modernc.org/sqlite` | CGO-free SQLite 驱动 |
| `golang.org/x/crypto` | bcrypt 密码哈希 |

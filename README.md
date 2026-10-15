# DNSLogHUB

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

一个使用 Go 编写的**多协议外带数据（Out-of-Band）检测平台**，用于在授权安全测试中捕获 DNS / HTTP / LDAP / RMI 回连，辅助验证 SSRF、盲注入、Log4j、Fastjson、XXE 等漏洞。

> 本项目基于 [lanyi1998/DNSlog-GO](https://github.com/lanyi1998/DNSlog-GO)（MIT License）二次开发并更名为 **DNSLogHUB**。

---

## 目录

- [项目定位](#项目定位)
- [核心功能](#核心功能)
- [适用场景](#适用场景)
- [技术栈](#技术栈)
- [目录结构](#目录结构)
- [环境要求](#环境要求)
- [配置说明](#配置说明)
- [编译教程](#编译教程)
- [部署教程](#部署教程)
- [启动与验证](#启动与验证)
- [升级与回滚](#升级与回滚)
- [常见问题](#常见问题)
- [安全与合规声明](#安全与合规声明)
- [许可证](#许可证)

---

## 项目定位

DNSLogHUB 是面向**授权渗透测试 / 安全评估**场景的自建 DNSLog 平台。它监听一个或多个域名的 DNS 解析请求，并复用 HTTP 端口捕获 HTTP / LDAP / RMI 回连，帮助测试者确认目标是否存在无回显的漏洞（盲注、SSRF、命令注入外带等）。

数据（回连记录、注册用户、邀请码）持久化到 SQLite，重启不丢失；内置多用户隔离、注册 + 邀请码、管理后台、中英双语与赛博朋克风格界面。

## 核心功能

- **多协议回连捕获**：DNS（A/TXT）、HTTP、LDAP、RMI/JRMI
- **多用户隔离**：token → 专属子域名前缀，互不干扰
- **多域名支持**：可配置多个根域名，并按用户分配所属域名
- **用户注册 + 邀请码**：邀请码 bcrypt 哈希存储，支持有效期 / 使用次数 / 绑定子域名 / 限流 / 审计
- **管理后台**：登录页（`/admin`）+ 独立后台页（`/admin/dashboard`），可管理邀请码、用户、配置热加载
- **SQLite 持久化**：WAL、事务、索引、版本化迁移、备份，记录 / 用户 / 邀请码全部落库
- **结构化审计日志**：滚动写入、按大小/数量/天数保留、token 脱敏、请求 ID 串联
- **配置热加载**：注册开关 / 注册限流可在线生效（SIGHUP 或管理接口触发）
- **i18n**：中 / 英双语，前端可切换
- **赛博朋克终端风格 UI**：暗色默认，明暗主题可切换，响应式布局
- **Go SDK**：`pkg/sdk` 提供单条 / 批量 DNS 验证等能力

## 适用场景

- 授权渗透测试中的 SSRF / 盲注入 / 命令注入外带探测
- Log4j / Fastjson / XXE 等漏洞的回连验证
- 替代 Burp Collaborator 的自建外带服务器
- 红蓝对抗演练中的隐蔽回连信道

> ⚠️ **仅限授权范围**：请勿在未授权的系统或网络上使用本工具。详见[安全与合规声明](#安全与合规声明)。

## 技术栈

| 层 | 技术 |
|---|---|
| 语言 | Go 1.26+ |
| HTTP | Gin |
| DNS | `golang.org/x/net/dns/dnsmessage`（无第三方 DNS 库） |
| 持久化 | SQLite（`modernc.org/sqlite`，纯 Go，无需 CGO） |
| 日志 | zap（结构化）+ 自研滚动写入器 |
| 密码哈希 | `golang.org/x/crypto/bcrypt` |
| LDAP 解析 | `go-asn1-ber/asn1-ber` |
| IP 归属 | 内置纯真 IP 库（qqwry） |
| 前端 | Vue2 + axios（`go:embed` 内嵌），CSS Variables 主题 |

## 目录结构

```
DNSLogHUB/
├── cmd/
│   ├── server/main.go            # 服务端入口
│   └── invitegen/main.go         # 邀请码生成工具
├── internal/
│   ├── config/                   # 配置结构、热加载、多域名、用户域映射
│   ├── dns/                      # DNS UDP 服务器（A/TXT 记录）
│   ├── handler/                  # HTTP 业务处理（API/管理/注册/用户自助）
│   ├── i18n/                     # 国际化（locales/*.json）
│   ├── ipwry/                    # 纯真 IP 库解析
│   ├── logger/                   # zap 日志 + 审计日志
│   ├── middleware/               # 鉴权（token / admin / request-id）
│   ├── model/                    # 内存数据模型
│   ├── router/                   # 路由 + 内嵌前端资源
│   │   └── resources/            # 前端模板 / css / js
│   └── store/                    # SQLite 持久化
├── pkg/
│   ├── httpHandle/               # 单连接 HTTP 服务器包装（多协议分流用）
│   ├── ldap/                     # LDAP 协议解析
│   ├── rmi/                      # JRMI 协议解析
│   └── sdk/                      # Go SDK
├── tests/api/                    # hurl 接口测试
├── config.yaml                   # 配置文件
├── build.sh / build.bat          # 交叉编译脚本
├── makefile                      # 测试入口
├── API.md                        # API 文档
└── go.mod
```

## 环境要求

**运行（服务器）**
- Linux（推荐 Ubuntu 20.04+ / Debian 11+），需能绑定 **UDP 53** 与 **TCP 8000**
- 一个指向该服务器的**根域名**（NS 记录指向服务器公网 IP）
- 公网 IP 与可配置的防火墙（放行 53/udp、8000/tcp）

**编译**
- Go 1.26+（`go.mod` 声明 `go 1.26.0`，`toolchain go1.26.9`）
- 交叉编译无需 CGO（`CGO_ENABLED=0`）

## 配置说明

所有配置集中在根目录 `config.yaml`，启动时读取一次（部分项支持热加载，见下）。

```yaml
Http:
  host: 0.0.0.0            # 监听地址（0.0.0.0 = 全部网卡）
  port: 8000               # HTTP/多协议对外端口（Web、/api、HTTP/LDAP/RMI 回调共用）
  consoleDisable: false    # true = 关闭内置前端，仅保留 API

User:                      # 静态用户：token -> 子域名前缀（重启生效）
  'admin': 'dnslog'

Dns:
  domain: your.domain      # 根域名（不带前导点/结尾点）
  domains: []              # 可选：多域名列表（含主域名），按用户分配所属域名
  ARecord: {}              # 静态 A 记录：<子域>: <IPv4>

Register:
  enabled: false           # 是否开启注册（true 时首页进入欢迎页）
  rateLimitPerMinute: 5    # 注册每 IP 每分钟限流（可热加载）

Log:
  level: info              # debug/info/warn/error
  dir: logs                # 日志目录（审计日志写入 logs/audit.log）
  maxSize: 10              # 单文件大小上限 (MB)
  maxBackups: 7            # 保留历史文件数
  maxAge: 30               # 历史文件最长保留天数

Data:
  dbFile: data/dnsloghub.db  # SQLite 数据库文件
  flushInterval: 10          # 内存记录批量落盘间隔（秒），退出时也会落盘

Admin:
  token: ""                # 管理员令牌（非空才启用 /api/admin/* 与后台页面）
  loginPath: /admin        # 后台登录页路径（可自定义）
  dashboardPath: /admin/dashboard  # 后台管理页路径（可自定义）
```

**关键说明**

- `User` 里的值是「子域名前缀」：`'admin': 'dnslog'` → 回调域名 `abc.dnslog.your.domain`。
- 静态用户与注册用户**分离**：注册用户写入 SQLite，静态用户写在此文件的 `User` 下；两者子域名都需唯一。
- `Admin.token` 仅在**启动时**读取，改动需重启；`Register.enabled` / `rateLimitPerMinute` 可热加载。
- `Admin.loginPath` / `dashboardPath` 可自定义后台页面路径，改动需重启。

## 编译教程

### 方式一：一键交叉编译（推荐）

```bash
# Linux / macOS / Git Bash
bash build.sh            # 产出 dist/ 下 6 平台二进制 + tar.gz + sha256
bash build.sh clean      # 清理 dist/

# Windows（cmd 或 PowerShell）
build.bat
build.bat clean
```

产物示例（`dist/`）：

```
dnsloghub-linux-amd64
dnsloghub-linux-amd64.tar.gz
dnsloghub-linux-amd64.tar.gz.sha256
invitegen-linux-amd64
...（另含 linux/arm64、windows/amd64、windows/arm64、darwin/amd64、darwin/arm64）
```

> 压缩包内包含 `dnsloghub`（服务端）、`invitegen`（邀请码工具）、`config.yaml`。

### 方式二：手动编译单平台

```bash
CGO_ENABLED=0 go build -o dnsloghub ./cmd/server
CGO_ENABLED=0 go build -o invitegen ./cmd/invitegen
```

编译时会通过 `ldflags` 注入版本号、commit、构建时间（启动日志可见）：

```bash
VERSION=v1.0.0 bash build.sh   # 自定义版本号
```

## 部署教程

以 **Ubuntu 22.04** 为例，其余 Linux 发行版同理。

### 1. 准备域名

在域名服务商处，将根域名的 **NS 记录**指向服务器公网 IP（部分服务商要求使用 `ns1.<ip>.nip.io` 之类的已认证 NS）。完成后用 `dig` 验证：

```bash
dig @你的服务器IP abc.dnslog.your.domain
```

### 2. 上传并解压

```bash
scp dist/dnsloghub-linux-amd64.tar.gz user@server:/opt/dnsloghub/
ssh user@server
cd /opt/dnsloghub
tar xzf dnsloghub-linux-amd64.tar.gz
chmod +x dnsloghub invitegen
```

### 3. 编辑配置

```bash
vi config.yaml
```

至少修改：

```yaml
Dns:
  domain: your.domain        # 改成你的根域名
Admin:
  token: "改成一个强随机串"   # 开启管理后台
Register:
  enabled: true             # 如需开放注册
```

生成强随机管理员 token：

```bash
openssl rand -hex 32
```

### 4. 解决 53 端口占用（Ubuntu/Debian 重点）

Ubuntu/Debian 默认启用 **systemd-resolved**，会监听 `127.0.0.53:53`，可能与本程序冲突。先检查占用：

```bash
sudo ss -lunp | grep :53
```

若看到 `systemd-resolve` / `127.0.0.53:53`，执行以下步骤处理：

```bash
# 0) 备份当前 DNS 配置（务必先备份）
sudo cp /etc/resolv.conf /etc/resolv.conf.bak

# 1) 停止并禁用 systemd-resolved
sudo systemctl stop systemd-resolved
sudo systemctl disable systemd-resolved

# 2) 删除旧软链接（Ubuntu 默认是链接文件，不删无法直接写内容）
sudo rm -f /etc/resolv.conf

# 3) 写入公共 DNS
cat <<EOF | sudo tee /etc/resolv.conf
nameserver 223.5.5.5
nameserver 114.114.114.114
EOF
```

**验证**：

```bash
cat /etc/resolv.conf                 # 应显示上面两个 nameserver
systemctl status systemd-resolved    # 应为 inactive (dead)
sudo ss -lunp | grep :53             # 53 端口应已释放
```

**若系统由 NetworkManager / resolvconf 管理 DNS**，直接改 `/etc/resolv.conf` 重启后可能被覆盖。此时二选一：

- **NetworkManager**：`sudo nmcli connection modify <连接名> ipv4.dns "223.5.5.5 114.114.114.114"` 再 `sudo nmcli connection up <连接名>`；
- 或安装/配置 `resolvconf` 后把 DNS 写入其上游配置，使其重启后仍保持。

> 确认操作**不影响当前网络与 DNS 解析**：若中途出现解析异常，执行 `sudo cp /etc/resolv.conf.bak /etc/resolv.conf` 恢复。

### 5. 绑定 53 端口

普通用户无法绑定 1024 以下端口，二选一：

```bash
# 方式 A：授予二进制绑定低端口能力（推荐，无需 root 运行）
sudo setcap cap_net_bind_service=+ep ./dnsloghub

# 方式 B：直接以 root 运行
sudo ./dnsloghub
```

### 6. 防火墙

```bash
sudo ufw allow 53/udp
sudo ufw allow 8000/tcp
```

## 启动与验证

### 前台启动

```bash
./dnsloghub
```

启动日志示例（含版本/commit/构建时间）：

```
{"level":"info","msg":"DNSLogHUB starting","version":"v1.0.0","commit":"abc1234","buildTime":"2026-10-16T00:00:00Z"}
{"level":"info","msg":"DNS Listing start success..."}
{"level":"info","msg":"Http Server start..."}
```

### systemd 开机自启（可选）

```ini
# /etc/systemd/system/dnsloghub.service
[Unit]
Description=DNSLogHUB
After=network.target

[Service]
WorkingDirectory=/opt/dnsloghub
ExecStart=/opt/dnsloghub/dnsloghub
Restart=always
RestartSec=5
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now dnsloghub
systemctl status dnsloghub
```

### 验证

```bash
# 1) 首页/API 可访问
curl http://127.0.0.1:8000/api/ip

# 2) token 校验（以 config.yaml 的静态用户为例）
curl -X POST http://127.0.0.1:8000/api/verifyToken \
  -H 'Content-Type: application/json' -d '{"token":"admin"}'

# 3) DNS 回连（在目标上解析 abc.dnslog.your.domain 后验证）
curl -X POST http://127.0.0.1:8000/api/verifyDns \
  -H 'token: admin' -H 'Content-Type: application/json' \
  -d '{"query":"abc.dnslog.your.domain"}'
```

### 访问管理后台

1. 浏览器打开 `http://<服务器IP>:8000/admin`（登录页）
2. 输入 `config.yaml` 中的 `Admin.token` → 提交
3. 自动跳转 `http://<服务器IP>:8000/admin/dashboard`（后台管理页）

管理后台可：查看/作废邀请码、查看/禁用/启用用户、改用户子域名、热加载配置。

### 生成邀请码（服务器上）

```bash
./invitegen -uses 1 -note "给张三" -db data/dnsloghub.db
# 输出的明文码立即通过安全渠道分发给注册用户
```

## 升级与回滚

### 升级前备份

```bash
cd /opt/dnsloghub
cp dnsloghub dnsloghub.bak
cp config.yaml config.yaml.bak
cp -r data data.bak        # 关键：SQLite 数据
```

### 升级

```bash
# 用新产物覆盖二进制（config.yaml 如需变更再覆盖）
cp /path/to/new/dnsloghub ./dnsloghub
sudo systemctl restart dnsloghub   # 或 kill 旧进程后重新 ./dnsloghub
```

> 数据库 schema 有版本化迁移（`PRAGMA user_version`），首次以新版本启动会自动迁移；建议升级前仍备份 `data/`。

### 回滚

```bash
cp dnsloghub.bak dnsloghub
cp config.yaml.bak config.yaml
rm -rf data && mv data.bak data
sudo systemctl restart dnsloghub
```

## 常见问题

**Q1：DNS 服务无法绑定 53 端口 / 收不到回连**
多半是 systemd-resolved 占用 53 端口，参见[部署教程第 4 步](#4-解决-53-端口占用ubuntudebian-重点)。先 `sudo ss -lunp | grep :53` 确认占用。

**Q2：重启后前端取不到数据**
前端以服务端为权威源，`localStorage` 缓存旧子域名时已会自动重新校验并拉取最新子域名。若仍异常，清空浏览器 `localStorage` 后重新登录即可。

**Q3：重启会不会丢记录？**
不会。记录/用户/邀请码均持久化到 `data/dnsloghub.db`（SQLite）。正常退出/`Ctrl+C` 零丢失；断电/强杀最多丢最后 `Data.flushInterval`（默认 10 秒）。

**Q4：管理页面 404**
确认 `Admin.token` 非空；确认访问的是 `Admin.loginPath`（默认 `/admin`）与 `Admin.dashboardPath`（默认 `/admin/dashboard`）；改动配置后需重启。

**Q5：注册功能用不了**
确认 `Register.enabled: true`；确认已用 `invitegen` 生成邀请码（写入 `data/dnsloghub.db`）。

**Q6：验证返回「Not Found」**
`verifyDns`/`verifyHttp` 是「查询是否存在回连」，「Not Found」表示尚未收到该回连（正常负结果），先确认目标确实发起了解析/请求。

**Q7：改了子域名后新子域名不生效**
`config.yaml` 的 `User` 是启动时读取，改动后需重启；注册用户的子域名可在后台「改子域」实时生效。

**Q8：如何开多域名**
在 `Dns.domains` 中列出多个根域名，注册时用户可选择所属域名；DNS 仅对用户「所属域名」的解析记录回连。

## 安全与合规声明

- 本工具仅用于**已获授权**的安全测试、红蓝对抗与自身资产检测，禁止用于任何未授权活动。
- 默认鉴权为**明文 token**（请求头 `token` / `X-Admin-Token`），公网部署务必置于 HTTPS 反向代理之后，避免凭据与回连报文被嗅探。
- 建议：`Admin.token` 使用强随机串；定期更换用户 token；限制 `8000` 端口访问来源；关注 `logs/audit.log` 审计日志。
- 回连报文（HTTP 请求）会明文存储于数据库并回显，请勿在目标侧触发含敏感凭据的回连。

## 许可证

本项目基于 [lanyi1998/DNSlog-GO](https://github.com/lanyi1998/DNSlog-GO) 二次开发，遵循原项目 **MIT License**（Copyright (c) 2020 lanyi），详见 [LICENSE](LICENSE)。二次开发部分的修改与新增同样以 MIT License 发布。

---

## 待补充确认

1. **根域名**：示例中的 `your.domain` 请替换为你的真实域名；仓库内 `config.yaml` 当前域名为 `dnslog.cn`，注释中的示例域名仍为 `log.skin`，建议一并统一。
2. **管理员 token**：`Admin.token` 当前为空（管理后台默认关闭），部署时务必填入强随机值。
3. **NS 服务商细节**：不同域名服务商对「NS 指向裸 IP」的支持程度不同，部分需配合 `nip.io` 等反向解析服务，请按你的服务商文档确认。

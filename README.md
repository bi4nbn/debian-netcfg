# Debian Netcfg 网络配置工具

[![Build netcfg Linux amd64](https://github.com/bi4nbn/debian-netcfg/actions/workflows/build.yml/badge.svg)](https://github.com/bi4nbn/debian-netcfg/actions/workflows/build.yml)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> 🚀 基于 Go 开发的 Debian 系交互式网络管理工具，专为远程 SSH 运维场景深度优化

## ✨ 项目简介

`netcfg` 是一款专为 Debian 系服务器设计的网络配置工具，完美适配传统 `/etc/network/interfaces` ifupdown 网络栈。工具采用交互式命令行界面，一站式解决：

- 🌐 单网卡静态/DHCP 配置
- 🔗 Bond 链路聚合（支持 802.3ad LACP）
- 🌍 独立 IPv6 配置
- 🔍 DNS 连通性测速
- ⚙️ 系统一键初始化

**核心优势：全程零 SSH 断连风险！**

---

## 🎯 核心特性

### 1. 🔒 SSH 远程零断连（项目最大亮点）

传统改 IP 方式会导致 SSH 断开，`netcfg` 通过以下机制彻底解决：

- **IPv4 热加载不重启网卡**：先新增双 IP 共存 → 延迟删除旧 IP → 无网络真空期
- **SSH 会话智能识别**：自动识别当前 SSH 出口网卡，清理时自动跳过
- **Bond ↔ 单网卡安全切换**：自动清理残留 IP，避免同 IP 多网卡冲突
- **IP 冲突自动检测**：配置新 IP 前自动检测并清除所有接口上的相同 IP

### 2. 🔗 Bond 链路聚合（实时生效）

支持 3 种工业级 Bond 模式：

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| `balance-rr` | 轮询负载均衡 | 流量均分多网卡 |
| `active-backup` | 主备故障转移 | 单链路工作，故障自动切换 |
| `802.3ad` | LACP 交换机聚合 | 带宽叠加，需交换机支持 |

**802.3ad LACP 实时参数：**
- `xmit_hash_policy=layer3+4`：基于源/目标 IP+端口哈希
- `lacp_rate=fast`：快速 LACP 协商
- 所有参数在 bond0 启动后**立即生效**，无需重启

### 3. 🌍 独立 IPv6 配置

- **完全不修改 IPv4 链路**：仅追加 IPv6 配置，零影响
- **在线热生效**：使用 ip 命令在线绑定，无 ifdown/ifup 操作
- **业务扩容首选**：在线生产环境安全扩展 IPv6

### 4. 🛡️ 多层安全保障

- **配置自动备份**：修改前生成带时间戳备份（如 `interfaces.bak_20260806_153022`）
- **语法预校验**：写入后执行 `ifup --no-act` 验证配置正确性
- **冲突服务自动停止**：NetworkManager / systemd-networkd 自动禁用
- **依赖检测**：iproute2 / ifupdown 缺失时提示并询问是否补装，ifenslave 在配置 Bond 时按需补装
- **配置合并写入**：只替换被管理网卡的 stanza，保留 `source` 指令与其它网卡配置

### 5. 🌐 完整中英双语支持

- 全量 i18n 国际化，无硬编码中英文混杂
- 主菜单一键切换语言，状态全局持久生效
- 支持 200+ 翻译条目，覆盖所有提示、日志、告警

### 6. ⚙️ 一键系统初始化

- 自动识别 Debian 版本（bullseye/bookworm/trixie）
- 替换 APT 源为华为云国内镜像
- 批量安装运维依赖：wget/curl/sudo/ifenslave
- SSH 安全加固：仅密钥登录、禁用密码、限制登录尝试

---

## 🔐 安全行为与可配置项

系统初始化（菜单第 5 项）会改写 APT 源与 SSH 配置，请先了解以下行为：

- **公钥为追加写入**：`/root/.ssh/authorized_keys` 采用"追加+去重"，**不会**清空服务器上已有的密钥。
- **关闭密码登录**：按设计写入 `PasswordAuthentication no`，仅允许公钥认证。
  可用 `NETCFG_KEEP_PASSWORD_AUTH=1` 临时保留密码登录。
  ⚠️ 执行前请确认自己持有 `NETCFG_SSH_PUBKEY` / `/etc/netcfg/ssh_pubkey` / 内置公钥所对应的**私钥**。
- **公钥来源可覆盖**（优先级从高到低）：
  1. 环境变量 `NETCFG_SSH_PUBKEY`（直接给出完整公钥行）
  2. 文件 `/etc/netcfg/ssh_pubkey`
  3. 源码内置默认公钥
- **可跳过 SSH 加固**：设置 `NETCFG_SKIP_SSH_HARDENING=1` 后，初始化只改 APT 源与装包，不动 SSH 配置。
- **sshd 配置回滚**：`sshd -t` 校验失败时会从真实备份路径恢复并报错，不会留下损坏的 `sshd_config`。
- **连通性测试不再隐式初始化**：菜单第 4 项只做 DNS 测试，如需初始化必须显式确认（默认不执行）。
- **语言持久化**：语言选择写入 `/etc/netcfg.lang`，重启后保留。
- **自更新校验**：菜单第 7 项下载后会校验 ELF 头与 SHA256
  （优先 `NETCFG_UPDATE_SHA256`，其次远程 `netcfg.sha256`），无校验值时必须人工确认；替换前自动备份原程序。

---

## 📦 系统兼容性

### ✅ 支持系统

- Debian 11 (bullseye)
- Debian 12 (bookworm)
- Debian 13 (trixie)
- 使用传统 `/etc/network/interfaces` 的 Ubuntu 系统

### ❌ 不兼容环境

- Netplan 作为主网络栈的系统（如新版 Ubuntu 桌面）
- NetworkManager 作为主网络栈的系统

### ⚠️ 运行要求

- **权限**：必须 root / sudo 权限执行
- **依赖**：编译为静态无 CGO 二进制，服务器无需预装 Go 环境

---

## 🚀 快速开始

### 1. 下载安装

```bash

# 从源码编译
git clone https://github.com/bi4nbn/debian-netcfg.git
cd debian-netcfg
./build.sh
cp netcfg /usr/local/bin/
```

### 2. 运行工具

```bash
# root 直接执行
netcfg

# 普通用户 sudo 提权
sudo netcfg
```

### 3. 主菜单功能

启动后自动展示所有物理网卡状态：

```text
======================================
  Debian 网络配置工具 v1.1.7
======================================
  1. 单网卡IP配置
  2. 网卡绑定链路聚合
  3. 单独配置IPv6
  4. 网络连通性测试
  5. 初始化系统
  6. 切换语言
  7. 更新脚本
  0. 退出
======================================

==== 当前网卡状态 ====
物理网卡列表：
  UP   eth0      IPv4: 192.168.1.10/24   IPv6: N/A
  DOWN eth1      IPv4: 无IP              IPv6: N/A
默认IPv4网关：192.168.1.1
默认IPv6网关：N/A
======================================
```

> 界面默认语言为英文（`1. English`），可在主菜单第 6 项切换为中文，选择结果持久化保存。

---

## 📖 功能详解

### 1. 单网卡 IP 配置

**使用场景**：配置单个物理网卡的 IPv4/IPv6 地址

**操作流程**：
1. 自动枚举物理网卡（过滤 lo/docker/veth/tun/bond 等虚拟接口）
2. 自动检测当前模式：DHCP / 静态 IP / 无地址
3. 统一 CIDR 格式输入（如 `192.168.1.10/24`），自动计算网关
4. 可选同步配置静态 IPv6
5. 热加载生效，零 SSH 断连

**关键特性**：
- 如果存在 bond0，**自动清理**（带 SSH 保护）
- 自动检测并清除 IP 冲突
- 自动清理其他网卡残留 IP

### 2. Bond 链路聚合

**使用场景**：多网卡绑定，提升带宽或冗余

**操作流程**：
1. 选择要绑定的物理网卡（支持多网卡）
2. 配置 IPv4 地址（CIDR 格式）
3. 选择 Bond 模式（默认 802.3ad LACP）
4. 可选配置 IPv6
5. 实时创建 bond0 并生效

**802.3ad LACP 配置**：
```bash
# 自动配置的参数
bond-mode 802.3ad
bond-miimon 100
bond-lacp-rate fast              # 快速协商
bond-xmit-hash-policy layer3+4   # 基于 IP+端口哈希
```

**验证 Bond 状态**：
```bash
# 查看 bond 运行模式
cat /proc/net/bonding/bond0 | grep "Bonding Mode"
# 输出: Bonding Mode: IEEE 802.3ad Dynamic link aggregation

# 查看 xmit_hash_policy
cat /proc/net/bonding/bond0 | grep "Transmit Hash Policy"
# 输出: Transmit Hash Policy: layer3+4 (1)
```

### 3. 独立 IPv6 配置

**使用场景**：在线业务扩展 IPv6，不影响现有 IPv4

**特点**：
- 仅追加 IPv6 配置到 interfaces 文件
- 使用 ip 命令在线绑定，无 ifdown/ifup
- 完全不修改 IPv4 地址和路由

### 4. DNS 连通性测试

**测试目标**：
- IPv4: 阿里云公共 DNS `223.5.5.5` / `223.6.6.6`
- IPv6: 阿里云公共 DNS `2400:3200::1` / `2400:3200:baba::1`

**输出示例**：
```text
==== DNS连通性测试 ====
测试IPv4公共DNS
  223.5.5.5 ... 连通正常 平均延迟：12.5 ms
  223.6.6.6 ... 连通正常 平均延迟：13.2 ms

测试IPv6公共DNS
  2400:3200::1 ... IPv6不可达
  2400:3200:baba::1 ... IPv6不可达
```

### 5. 系统初始化

**功能**：
- 替换 APT 源为华为云国内镜像（改写前备份）
- 安装基础依赖：wget/curl/sudo/ifenslave
- SSH 加固：公钥**追加**写入、`AllowTcpForwarding yes`、限制登录尝试
  - 按设计关闭密码登录（`PasswordAuthentication no`），可用 `NETCFG_KEEP_PASSWORD_AUTH=1` 保留
  - 可用 `NETCFG_SKIP_SSH_HARDENING=1` 跳过 SSH 部分
- 写入 `/etc/netcfg.initialized` 标记防止重复执行

> 菜单第 4 项的连通性测试**不会**自动触发初始化，需要显式确认（默认不执行）。

---

## 🔧 SSH 防断连核心机制

### 1. SSH 会话识别

```go
// 读取 SSH_CONNECTION 环境变量
conn := os.Getenv("SSH_CONNECTION")
// 格式: 客户端IP 客户端端口 服务端IP 服务端端口
// 示例: 192.168.1.100 54321 192.168.1.10 22

// 通过路由表识别出口网卡
ip route get 192.168.1.100
# 输出: 192.168.1.100 dev eth0 src 192.168.1.10
```

### 2. IPv4 平滑切换流程

```text
步骤1: 新 IP 附加到网卡（新旧 IP 双地址共存）
       ip addr add 192.168.1.20/24 dev eth0

步骤2: 添加新网关高优先级路由
       ip route add default via 192.168.1.1 dev eth0 metric 100

步骤3: 删除旧默认网关
       ip route del default via 192.168.1.1

步骤4: 提升新路由优先级
       ip route change default via 192.168.1.1 dev eth0 metric 0

步骤5: 延迟 1 秒后删除旧 IP
       ip addr del 192.168.1.10/24 dev eth0
```

### 3. Bond ↔ 单网卡切换清理

**从 Bond 切换到单网卡**：
1. 检测是否存在 bond0
2. 添加 SSH 回程路由保护
3. 清除 bond0 的所有 IP 地址
4. 如果 SSH 不通过 bond0，删除接口
5. 配置新 IP（无冲突）

**从单网卡切换到 Bond**：
1. 清理旧 bond0 残留配置
2. 创建新 bond0
3. 自动清空所有 slave 网卡三层 IP
4. 仅 bond0 保留业务地址

---

## 📂 配置文件说明

| 文件路径 | 说明 | 备份策略 |
|----------|------|----------|
| `/etc/network/interfaces` | 主网络配置（块级替换写入） | 每次修改前生成 `.bak_YYYYMMDD_HHMMSS` |
| `/etc/resolv.conf` | DNS 配置 | 修改前生成备份，保留原有非阿里云 DNS |
| `/etc/apt/sources.list` | APT 源（初始化时改写） | 初始化前生成备份 |
| `/etc/ssh/sshd_config` | SSH 配置（初始化时改写） | 初始化前生成备份，校验失败自动回滚 |
| `/etc/modules-load.d/bonding.conf` | Bond 内核模块开机自启 | Bond 模式自动创建，切换单网卡自动删除 |
| `/etc/netcfg.initialized` | 系统初始化标记 | 防止重复执行初始化 |
| `/etc/netcfg.lang` | 语言偏好持久化 | 切换语言时写入 |
| `/etc/netcfg/ssh_pubkey` | 可选：自定义待下发公钥 | 存在时优先于内置公钥使用 |

---

## 🛠️ 开发指南

### 项目结构

```text
debian-netcfg/
├── main.go          # 程序入口、主菜单循环
├── common.go        # 公共工具：日志、输入、IP校验、SSH保护、依赖检查、自更新
├── network.go       # 核心网络层：网卡枚举、IP热加载、interfaces 块级写入
├── bond.go          # Bond 链路聚合完整实现
├── single.go        # 单网卡配置逻辑
├── ipv6.go          # 独立 IPv6 配置模块
├── dnstest.go       # DNS 连通性测速
├── init.go          # 系统初始化：APT源、SSH加固
├── i18n.go          # 中英双语国际化（200 条目，中英 key 完全对齐）
├── network_test.go  # interfaces 读写相关单元测试
├── init_test.go     # SSH 公钥合并/校验相关单元测试
├── build.sh         # 静态编译 + UPX 压缩
├── push.sh          # 一键发布：测试、提交、打标签、推送、编译
├── go.mod           # Go 模块定义
├── LICENSE          # MIT 协议
└── README.md        # 本文档
```

### 代码统计

```text
文件             行数    职责
network.go        816    核心网络层
common.go         622    公共工具库
i18n.go           476    国际化字典
init.go           345    系统初始化
bond.go           259    Bond 配置
network_test.go   214    单元测试（interfaces 读写）
single.go         200    单网卡配置
init_test.go      162    单元测试（SSH 公钥处理）
main.go           141    程序入口
dnstest.go         87    DNS 测试
ipv6.go            78    IPv6 配置
─────────────────────────────
总计             3400    Go 代码（含测试）
```

### 测试

```bash
go vet ./...
go test ./...      # 覆盖 interfaces 块级替换、IPv6 追加、authorized_keys 合并、校验值解析
```

### 编译命令

```bash
# 本地编译（推荐开发调试）
./build.sh

# 简易编译（无 Git 环境）
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o netcfg
chmod +x ./netcfg

# 一键推送 + 编译
bash push.sh
```

### 版本管理

```bash
# 版本号规则
v主版本.次版本.patch
# 示例: v1.0.2

# 自动递增 patch 号
bash push.sh
# 1. git add .
# 2. 输入 commit 备注
# 3. 自动创建新标签 v1.0.3
# 4. 推送到 GitHub
# 5. 自动编译二进制
```

---

## 🚨 常见问题

### Q: 运行提示权限不足？

A: 工具必须使用 root 用户或 sudo 提权执行：
```bash
sudo netcfg
```

### Q: 系统使用 Netplan/NetworkManager 无法使用？

A: 本工具基于传统 ifupdown 网络栈，需切换为 `/etc/network/interfaces` 管理网络。

### Q: 切换 Bond/单网卡时 SSH 会不会断开？

A: 绝大多数在线操作使用 ip 热加载不会断连。特殊情况：
- 如果 SSH 通过 bond0，bond0 接口保留但 IP 清除
- 用户需手动切换 SSH 到新网卡后清理 bond0

### Q: 服务器重启后网络配置丢失？

A: 配置永久写入 `/etc/network/interfaces`，正常重启保留。如果 NetworkManager 自动覆盖，需执行初始化脚本关闭冲突服务。

### Q: 802.3ad LACP 不生效？

A: 需要交换机也启用 LACP 协商。检查：
```bash
# 查看 bond 模式
cat /proc/net/bonding/bond0 | grep "Bonding Mode"

# 查看 LACP 状态
cat /proc/net/bonding/bond0 | grep "LACP rate"
```

### Q: 如何查看当前工具版本？

A: 直接执行 `netcfg`，主菜单标题第一行展示内置版本号。

### Q: IP 冲突如何处理？

A: 工具会自动检测并清除冲突 IP：
1. 配置新 IP 前检测所有接口
2. 自动清除其他接口上的相同 IP
3. 显示提示信息告知用户

### Q: 执行系统初始化会不会把我锁在 SSH 外面？

A: 初始化会**关闭密码登录**（这是设计意图），因此请先确认自己持有对应私钥。已做的保护：
1. `authorized_keys` 为**追加去重**写入，不会删除你已有的公钥（也会一并保留）；
2. 只有在确认公钥确实写入 `authorized_keys` 之后才会关闭密码登录；写入异常时强制保留 `yes`；
3. `sshd -t` 校验失败时自动从备份恢复 `sshd_config`，不会留下损坏配置。

临时保留密码登录：`NETCFG_KEEP_PASSWORD_AUTH=1`；完全跳过 SSH 加固：`NETCFG_SKIP_SSH_HARDENING=1`。

### Q: 工具会覆盖我原有的 interfaces / DNS 配置吗？

A: 不会整体覆盖：
- `interfaces` 采用块级替换，只重写被管理网卡的 stanza，保留 `source` 指令与其它网卡；
- `resolv.conf` 以阿里云 DNS 优先，原有其它（内网）DNS 追加保留；
- 所有改动前都会生成带时间戳的备份。

---

## 📋 配置示例

### 单网卡静态 IP

```text
# /etc/network/interfaces
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 192.168.1.10
    netmask 255.255.255.0
    gateway 192.168.1.1

iface eth0 inet6 static
    address 2409::1/64
    gateway 2409::1
```

### Bond 802.3ad LACP

```text
# /etc/network/interfaces
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet manual
    bond-master bond0

auto eth1
iface eth1 inet manual
    bond-master bond0

auto bond0
iface bond0 inet static
    address 192.168.1.10
    netmask 255.255.255.0
    gateway 192.168.1.1
    dns-nameservers 223.5.5.5 223.6.6.6
    bond-mode 802.3ad
    bond-miimon 100
    bond-slaves eth0 eth1
    bond-lacp-rate fast
    bond-xmit-hash-policy layer3+4

iface bond0 inet6 static
    address 2409::1/64
    gateway 2409::1
```

### DHCP 模式

```text
# /etc/network/interfaces
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
```

---

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

### 开发环境

```bash
# 克隆项目
git clone https://github.com/bi4nbn/debian-netcfg.git
cd debian-netcfg

# 安装依赖
go mod download

# 编译测试
./build.sh

# 运行测试
sudo ./netcfg
```

### 提交规范

```bash
# 功能开发
git commit -m "feat: 添加 XXX 功能"

# Bug 修复
git commit -m "fix: 修复 XXX 问题"

# 文档更新
git commit -m "docs: 更新 README"

# 代码重构
git commit -m "refactor: 重构 XXX 模块"
```

---

## 📄 开源协议

本项目采用 MIT 协议开源，详见 [LICENSE](LICENSE) 文件。

---

## 🙏 致谢

- [华为云 Debian APT 镜像源](https://mirrors.huaweicloud.com)
- [阿里云公共 DNS](https://dns.alidns.com)
- [Debian 社区 ifupdown](https://wiki.debian.org/NetworkConfiguration)
- 所有测试、反馈工具的运维同仁

---

## 📞 联系方式

- 作者：bi4nbn
- 邮箱：bi4nbn@qq.com
- GitHub：[https://github.com/bi4nbn/debian-netcfg](https://github.com/bi4nbn/debian-netcfg)

---

**⭐ 如果这个工具对你有帮助，请给个 Star 支持一下！**

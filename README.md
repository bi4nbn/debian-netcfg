# Debian Netcfg 网络配置工具

## 项目简介

`netcfg` 是基于 Go 开发、适配 Debian 系（ifupdown 网络栈）的交互式网络管理工具，专为远程 SSH 运维场景优化。它解决了单网卡静态/DHCP、Bond 链路聚合、独立 IPv6 配置等需求，全程尽量避免 SSH 连接中断，统一 IP 输入/网卡检测逻辑，消除了多菜单规则不一致、IP 多网卡重复残留等问题。

---

## 核心特性

### 1. 统一标准化交互逻辑
- 单网卡 / Bond / 独立 IPv6 三套功能复用同一套 IP、掩码、网关、IPv6 输入校验函数，无重复代码、规则统一
- 全局统一网卡模式检测（DHCP/静态/无地址），一处修改全功能生效

### 2. SSH 远程连接零断连优化（核心优势）
- **IPv4 热生效**：先新增 IP、延迟删除旧 IP；网关采用先加新路由再删旧路由，无网络真空期
- **自动识别当前 SSH 会话出口**：清理冗余网卡 IP 时自动跳过当前业务网卡，防止断连
- **Bond ↔ 单网卡双向清理**：切换 Bond 或单网卡时自动清理另一套接口 IP，仅保留当前业务网卡地址，杜绝同 IP 多网卡冲突
- **独立 IPv6 配置**：仅操作 IPv6 协议栈，完全不触碰 IPv4 会话

### 3. 完整配置自动清理机制
- 配置 Bond 后自动清空所有从网卡三层 IP（Bond 标准规范，slave 仅二层转发）
- 配置单网卡后自动销毁 bond0 接口、清空 bond 所有 IP（SSH 走 bond 时给出提示，不强制清理）
- 清理其他物理网卡冗余 IP，仅保留当前配置网卡业务地址

### 4. 国际化双语支持
- 完整中英双语词条，无硬编码中文/英文混杂，一键切换语言
- 所有日志、提示、警告、校验文本全部托管 i18n，环境语言统一输出

### 5. 配置安全保障
- 修改 `/etc/network/interfaces`、`resolv.conf` 前自动生成带时间戳备份文件
- 写入配置后语法预校验（`ifup --no-act`），语法异常可选择终止配置
- 自动安装缺失依赖：`iproute2` / `ifupdown` / `ifenslave`
- 自动停止冲突网络服务 NetworkManager / systemd-networkd

### 6. 附加工具功能
- DNS 连通性测速（阿里云公共 IPv4/IPv6 DNS）
- 网卡状态总览（主菜单展示所有网卡 UP/DOWN、IP、默认网关）
- 支持 3 种 Bond 模式：balance-rr / active-backup / 802.3ad(LACP)

### 7. 版本号自动注入
- 编译时从 Git 标签自动获取版本号并注入二进制
- 运行程序主菜单标题直接显示当前版本，方便运维识别
- 推送脚本自动递增 patch 版本号并打标签

### 8. 一键发布与部署脚本
- `push.sh`：自动提交代码、递增 patch 版本号、打标签、推送远程、编译二进制
- `ftp.sh`：一键将编译好的二进制上传到远程 FTP 服务器
- 流程完整闭环，便于持续迭代交付

---

## 系统适配

- **系统**：Debian 9/10/11/12、Ubuntu 使用 `ifupdown` 传统网络栈（不兼容 Netplan/NetworkManager 主栈环境）
- **权限**：必须 root 用户运行
- **依赖**：编译静态二进制，运行环境无需 Go 环境，仅系统自带基础命令

---

## 文件目录结构
debian-netcfg/
├── common.go 公共工具、日志、输入、IP校验、SSH检测、路由保护函数
├── network.go 网卡操作、IP热加载、bond清理、DNS写入、配置文件生成
├── single.go 单网卡静态/DHCP配置逻辑
├── bond.go 网卡链路聚合配置逻辑
├── ipv6.go 仅追加IPv6、不改动IPv4配置
├── dnstest.go DNS连通性测试工具（含自动初始化触发逻辑）
├── main.go 程序入口、主菜单、系统概览展示
├── i18n.go 中英双语国际化词条
├── init.go 系统初始化模块（写入华为云源、安装依赖、配置SSH）
├── build.sh 静态编译脚本（自动注入版本号，UPX压缩）
├── push.sh 一键推送脚本（提交代码、打标签、编译、推送）
├── ftp.sh 上传二进制到远程FTP服务器
├── init.sh 独立系统初始化脚本（Shell版本，可选备用）
├── go.mod Go模块定义
└── README.md 项目文档

text

---

## 编译方式

### 1. 本地编译（推荐）

```bash
# 编译并自动注入版本号（从 Git 标签获取）
./build.sh

# 或手动编译（版本号为 dev）
CGO_ENABLED=0 go build -o netcfg && chmod +x ./netcfg
生成 netcfg 单文件二进制，可直接拷贝到 Debian 服务器使用。

2. 一键推送并编译
bash
./push.sh
自动 git add . 并提交

自动检测最新标签，patch 号 +1 并打新标签

自动调用 build.sh 编译，版本号与标签一致

推送代码和标签到远程仓库

使用方法
1. 上传并赋予执行权限
bash
chmod +x ./netcfg
mv ./netcfg /usr/local/bin/
2. 运行工具（必须 root）
bash
sudo netcfg
# 或直接 root 执行
./netcfg
3. 快速上传到生产服务器（使用 ftp.sh）
bash
./ftp.sh
自动检查本地 netcfg 是否存在

上传到指定 FTP 服务器，远程文件名固定为 netcfg

如需保留历史版本，可手动备份或使用带时间戳的备份策略

主菜单功能说明
text
1. Single NIC IP Configuration   单网卡IP配置（DHCP/静态IPv4+可选IPv6）
2. NIC Bonding / Link Aggregation 网卡链路聚合（Bond）
3. Standalone IPv6 Config        仅给现有网卡追加IPv6，不改动IPv4
4. Network Connectivity Test     DNS连通性测速
5. Initialize System             系统初始化（APT源/依赖/SSH）
6. Switch Language               切换中文/英文界面
7. Update Script                 在线更新工具自身
0. Exit                          退出工具
功能详细说明
1. 单网卡配置
自动枚举所有物理网卡，过滤 lo/docker/veth/tun/br 等虚拟接口

自动识别网卡当前模式：DHCP / 静态IP / 无地址

DHCP：可选切换静态或保留自动获取

静态：可复用现有参数或重新输入

无地址：直接进入静态输入流程

IPv4 支持 CIDR 数字（24）或点分掩码（255.255.255.0）自动解析

可选同步配置静态 IPv6 地址（带 CIDR+网关）

确认写入后：

备份 /etc/network/interfaces
热加载 IP 不重启网卡，保障 SSH 不断
自动清理其他物理网卡 IP
若存在 bond0 且 SSH 不走 bond，自动销毁 bond0 接口
写入阿里云 DNS，备份 resolv.conf
最终输出完整网卡、IP、网关、DNS 校验信息
2. Bond 链路聚合
支持多物理网卡绑定，3 种工作模式：

balance-rr 轮询负载均衡

active-backup 主备故障转移

802.3ad LACP 交换机聚合

自动加载 bond 内核模块、写入开机自启配置

所有 slave 网卡自动设为 manual 二层模式，启动 bond0 后清空所有从网卡三层 IP

双向清理：已有单网卡 IP 会保留 bond0 唯一业务地址

支持同步配置 IPv6 静态地址

配置完成自动 ping 网关校验连通性

3. 独立 IPv6 配置
专为在线业务设计，完全不修改、不重启 IPv4

仅追加 IPv6 配置到现有网卡配置文件

使用在线 IPv6 地址绑定，不执行 ifdown/ifup

不会清理/改动任何 IPv4 地址、路由，生产在线扩容 IPv6 首选

4. DNS 连通测试
自动测试阿里云公共 IPv4/IPv6 DNS，输出平均延迟、连通状态：

IPv4: 223.5.5.5 / 223.6.6.6

IPv6: 2400:3200::1 / 2400:3200:baba::1

首次 IPv4 DNS 连通且系统未初始化时，会自动触发系统初始化（仅一次，通过 /etc/netcfg.initialized 标记文件持久化）。

5. 系统初始化（Initialize System）
自动识别 Debian 版本代号（bullseye/bookworm/trixie）

替换 APT 源为华为云镜像

执行 apt update 并安装 wget curl sudo ifenslave

配置 SSH：写入指定公钥、锁定 sshd_config（仅密钥登录、禁用密码）

执行成功后，写入 /etc/netcfg.initialized 标记，确保后续不再重复执行

6. 切换语言
支持中英双语一键切换，当前语言存储在 currentLang 全局变量中。

7. 更新脚本
从 https://bash.niteng.net/netcfg 下载最新版本并替换自身，自动重启。

安全机制说明（SSH核心保障）
1. SSH 会话识别
通过环境变量 SSH_CONNECTION 获取客户端 IP，查询路由出口网卡，区分当前业务网卡，清理冗余时跳过。

2. IPv4 平滑切换流程
添加新 IP 到网卡（双 IP 共存）

新增高优先级默认网关路由

删除旧默认路由，提升新路由优先级

延迟 1 秒确认网络稳定，删除旧 IP

3. 接口双向清理规则
切 Bond：清空所有 slave 物理网卡 IP

切单网卡：清空其他物理网卡 + 销毁 bond0（SSH 不在 bond 才执行）

4. 降级策略
所有重启网卡操作（ifdown/ifup）为降级兜底策略，优先使用在线热加载。

配置文件说明
1. /etc/network/interfaces
工具自动生成标准化配置，区分 lo、单网卡、bond、IPv4/IPv6 静态，重启永久生效。每次写入自动生成备份：interfaces.bak_20260710_004925

2. /etc/resolv.conf
默认写入阿里云公共 DNS，支持 resolvectl（systemd-resolved）与直接写入两种模式，同样生成时间戳备份。

3. Bond 开机模块
/etc/modules-load.d/bonding.conf 写入 bonding，开机自动加载内核模块，切换单网卡时自动删除。

4. 初始化标记文件
/etc/netcfg.initialized 记录初始化状态，防止 DNSTest 自动触发重复初始化。

版本号管理
编译时通过 -ldflags "-X main.Version=$VERSION" 注入版本号

版本号来源于 git describe：

有标签：v1.0.1

标签后有提交：v1.0.1-2-gabc123

工作区有未提交改动：v1.0.1-dirty

push.sh 自动递增 patch 号并打标签，版本号与 Git 标签同步

常见问题
Q：执行提示权限不足？
A：必须使用 root / sudo 运行。

Q：系统使用 Netplan / NetworkManager 无法使用？
A：本工具基于传统 ifupdown，需切换为 /etc/network/interfaces 网络栈。

Q：切换配置 SSH 不会断吗？
A：绝大多数场景不会；仅当你当前 SSH 走 bond，且强制销毁 bond 时会提示手动清理。

Q：重启后配置丢失？
A：配置写入系统永久 interfaces 文件，正常重启保留；若有 NetworkManager 覆盖需禁用冲突服务。

Q：IPv6 网关警告但不影响业务？
A：部分内网无 IPv6 网关属于正常现象，工具仅告警不阻断配置。

Q：如何查看当前版本号？
A：运行 ./netcfg，主菜单标题第一行会显示版本号（如 Debian 网络配置工具 v1.0.1）。

Q：更新脚本后版本号会丢失吗？
A：不会。版本号固化在二进制内部，更新后新版本会显示新标签对应的版本号。

Q：可以手动执行初始化吗？会重复执行吗？
A：主菜单选项 5 可随时手动执行初始化。DNSTest 中的自动初始化受 /etc/netcfg.initialized 标记文件保护，仅执行一次。

开源与生产使用说明
纯静态 Go 二进制，无第三方依赖，生产服务器直接部署

所有网络操作前置备份，配置异常可回滚 bak 文件

适配机房批量 Debian 服务器远程运维，规避改 IP 断连风险

代码逻辑统一、无碎片化菜单规则，便于后续扩展新功能

一键推送脚本 push.sh + 一键上传脚本 ftp.sh，实现“提交→编译→发布→部署”全链路闭环

特别鸣谢
华为云镜像站（APT 源加速）

阿里云公共 DNS（IPv4/IPv6）

Debian 社区与 ifupdown 网络栈

所有使用和反馈该工具的同仁
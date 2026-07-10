# Debian Netcfg 网络配置工具
## 项目简介
`netcfg` 是基于 Go 开发、适配 Debian 系（ifupdown 网络栈）的交互式网络管理工具，专为远程 SSH 运维场景优化，解决单网卡静态/DHCP、Bond 链路聚合、独立 IPv6 配置需求，全程尽可能避免 SSH 连接中断，统一 IP 输入/网卡检测逻辑，消除多菜单规则不一致、IP 多网卡重复残留等问题。

### 核心特性
1. **统一标准化交互逻辑**
   - 单网卡 / Bond / 独立 IPv6 三套功能复用同一套 IP、掩码、网关、IPv6 输入校验函数，无重复代码、规则统一
   - 全局统一网卡模式检测（DHCP/静态/无地址），一处修改全功能生效
2. **SSH 远程连接零断连优化（核心优势）**
   - IPv4 热生效：先新增 IP、延迟删除旧 IP；网关采用先加新路由再删旧路由，无网络真空期
   - 自动识别当前 SSH 客户端出口网卡，清理冗余网卡 IP 时自动跳过当前业务网卡，防止断连
   - Bond 切换单网卡 / 单网卡切换 Bond 双向自动清理另一套接口 IP，仅保留当前业务网卡地址，杜绝同IP多网卡冲突
   - 独立 IPv6 配置仅操作 IPv6 协议栈，完全不触碰 IPv4 会话
3. **完整配置自动清理机制**
   - 配置 Bond 后自动清空所有从网卡三层IP（Bond标准规范，slave仅二层转发）
   - 配置单网卡后自动销毁 bond0 接口、清空bond所有IP（SSH走bond时给出提示，不强制清理）
   - 清理其他物理网卡冗余IP，仅保留当前配置网卡业务地址
4. **国际化双语支持**
   - 完整中英双语词条，无硬编码中文/英文混杂，一键切换语言
   - 所有日志、提示、警告、校验文本全部托管i18n，环境语言统一输出
5. **配置安全保障**
   - 修改 `/etc/network/interfaces`、`resolv.conf` 前自动生成带时间戳备份文件
   - 写入配置后语法预校验（`ifup --no-act`），语法异常可选择终止配置
   - 自动安装缺失依赖：`iproute2` / `ifupdown` / `ifenslave`
   - 自动停止冲突网络服务 NetworkManager / systemd-networkd
6. **附加工具功能**
   - DNS 连通性测速（阿里云公共 IPv4/IPv6 DNS）
   - 网卡状态总览（开机主界面展示所有网卡UP/DOWN、IP、默认网关）
   - 支持 3 种 Bond 模式：balance-rr / active-backup / 802.3ad(LACP)

## 系统适配
- 系统：Debian 9/10/11/12、Ubuntu 使用 `ifupdown` 传统网络栈（不兼容 Netplan/NetworkManager 主栈环境）
- 权限：必须 root 用户运行
- 依赖：编译静态二进制，运行环境无需 Go 环境，仅系统自带基础命令

## 文件目录结构
```
debian-netcfg/
├── common.go     公共工具、日志、输入、IP校验、SSH检测、路由保护函数
├── network.go    网卡操作、IP热加载、bond清理、DNS写入、配置文件生成
├── single.go     单网卡静态/DHCP配置逻辑
├── bond.go       网卡链路聚合配置逻辑
├── ipv6.go       仅追加IPv6、不改动IPv4配置
├── dnstest.go    DNS连通性测试工具
├── main.go       程序入口、主菜单、系统概览展示
├── i18n.go       中英双语国际化词条
├── build.sh      静态编译脚本
└── README.md     项目文档
```

## 编译方式
1. 安装 golang 编译环境（编译机器）
2. 进入项目目录执行编译脚本：
```bash
# 静态编译，无CGO，可直接拷贝到服务器运行
CGO_ENABLED=0 go build -o netcfg && chmod +x ./netcfg
```
生成 `netcfg` 单文件二进制，直接上传到 Debian 服务器 `/usr/local/bin` 即可使用。

## 使用方法
### 1. 上传并赋予执行权限
```bash
chmod +x ./netcfg
mv ./netcfg /usr/local/bin/
```
### 2. 运行工具（必须root）
```bash
sudo netcfg
# 或直接root执行
./netcfg
```
### 主菜单功能说明
```
1. Single NIC IP Configuration   单网卡IP配置（DHCP/静态IPv4+可选IPv6）
2. NIC Bonding / Link Aggregation 网卡链路聚合
3. Standalone IPv6 Config        仅给现有网卡追加IPv6，不改动IPv4
4. Network Connectivity Test     DNS连通测速
5. Switch Language               切换中文/英文界面
0. Exit                          退出工具
```

## 功能详细说明
### 1. 单网卡配置
- 自动枚举所有物理网卡，过滤lo/docker/veth/tun/br等虚拟接口
- 自动识别网卡当前模式：DHCP / 静态IP / 无地址
  - DHCP：可选切换静态或保留自动获取
  - 静态：可复用现有参数或重新输入
  - 无地址：直接进入静态输入流程
- IPv4 支持CIDR数字(24)或点分掩码(255.255.255.0)自动解析
- 可选同步配置静态IPv6地址（带CIDR+网关）
- 确认写入后：
  1. 备份 `/etc/network/interfaces`
  2. 热加载IP不重启网卡，保障SSH不断
  3. 自动清理其他物理网卡IP
  4. 若存在bond0且SSH不走bond，自动销毁bond0接口
  5. 写入阿里云DNS，备份resolv.conf
  6. 最终输出完整网卡、IP、网关、DNS校验信息

### 2. Bond 链路聚合
支持多物理网卡绑定，3种工作模式：
1. balance-rr 轮询负载均衡
2. active-backup 主备故障转移
3. 802.3ad LACP交换机聚合
- 自动加载bond内核模块、写入开机自启配置
- 所有slave网卡自动设为manual二层模式，启动bond0后清空所有从网卡三层IP
- 双向清理：已有单网卡IP会保留bond0唯一业务地址
- 支持同步配置IPv6静态地址
- 配置完成自动ping网关校验连通性

### 3. 独立IPv6配置
**专为在线业务设计，完全不修改、不重启IPv4**
- 仅追加IPv6配置到现有网卡配置文件
- 使用在线IPv6地址绑定，不执行ifdown/ifup
- 不会清理/改动任何IPv4地址、路由，生产在线扩容IPv6首选

### 4. DNS 连通测试
自动测试阿里云公共IPv4/IPv6 DNS，输出平均延迟、连通状态：
IPv4: 223.5.5.5 / 223.6.6.6
IPv6: 2400:3200::1 / 2400:3200:baba::1

## 安全机制说明（SSH核心保障）
1. **SSH 会话识别**
   通过环境变量 `SSH_CONNECTION` 获取客户端IP，查询路由出口网卡，区分当前业务网卡，清理冗余时跳过。
2. **IPv4 平滑切换流程**
   1. 添加新IP到网卡（双IP共存）
   2. 新增高优先级默认网关路由
   3. 删除旧默认路由，提升新路由优先级
   4. 延迟1秒确认网络稳定，删除旧IP
3. **接口双向清理规则**
   - 切Bond：清空所有slave物理网卡IP
   - 切单网卡：清空其他物理网卡 + 销毁bond0（SSH不在bond才执行）
4. 所有重启网卡操作（ifdown/ifup）为降级兜底策略，优先使用在线热加载。

## 配置文件说明
### 1. /etc/network/interfaces
工具自动生成标准化配置，区分lo、单网卡、bond、IPv4/IPv6静态，重启永久生效。
每次写入自动生成备份：`interfaces.bak_20260710_004925`

### 2. /etc/resolv.conf
默认写入阿里云公共DNS，支持resolvectl（systemd-resolved）与直接写入两种模式，同样生成时间戳备份。

### 3. Bond 开机模块
`/etc/modules-load.d/bonding.conf` 写入bonding，开机自动加载内核模块，切换单网卡时自动删除。

## 常见问题
1. Q：执行提示权限不足
A：必须使用 root / sudo 运行
2. Q：系统使用 Netplan / NetworkManager 无法使用？
A：本工具基于传统 ifupdown，需切换为 `/etc/network/interfaces` 网络栈
3. Q：切换配置SSH不会断吗？
A：绝大多数场景不会；仅当你当前SSH走bond，且强制销毁bond时会提示手动清理
4. Q 重启后配置丢失？
A：配置写入系统永久interfaces文件，正常重启保留；若有NetworkManager覆盖需禁用冲突服务
5. Q：IPv6网关警告但不影响业务
A：部分内网无IPv6网关属于正常现象，工具仅告警不阻断配置

## 开源与生产使用说明
- 纯静态Go二进制，无第三方依赖，生产服务器直接部署
- 所有网络操作前置备份，配置异常可回滚bak文件
- 适配机房批量Debian服务器远程运维，规避改IP断连风险
- 代码逻辑统一、无碎片化菜单规则，便于后续扩展新功能
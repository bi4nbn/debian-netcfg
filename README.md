# Debian Netcfg 网络配置工具
## 项目简介
`netcfg` 是基于 Go 开发、适配 Debian 系（ifupdown 传统网络栈）的交互式网络管理工具，专为远程 SSH 运维场景深度优化。一站式解决单网卡静态/DHCP、Bond 链路聚合、独立 IPv6 配置、DNS 测速、系统初始化等运维需求，全程规避 SSH 会话中断风险，统一网卡、IP 校验逻辑，消除多网卡 IP 残留、配置冲突等常见机房运维问题。

## 核心特性
### 1. 统一标准化交互逻辑
- 单网卡 / Bond / 独立 IPv6 功能复用同一套 IP、掩码、网关、IPv6 校验输入函数，代码无冗余、配置规则完全统一
- 全局统一网卡模式自动检测（DHCP/静态/无地址），一处修改全功能同步生效

### 2. SSH 远程零断连核心优化（项目最大优势）
- **IPv4 热加载不重启网卡**：先新增双IP共存、延迟删除旧IP；网关采用新增高优路由再删除旧网关，无网络真空期
- 自动识别当前 SSH 会话出口网卡：清理冗余网卡IP时自动跳过业务网卡，杜绝远程断连
- Bond ↔ 单网卡双向自动清理：切换模式时自动清空另一套接口三层IP，仅保留业务地址，避免同IP多网卡冲突
- 独立 IPv6 配置模块：仅操作 IPv6 协议栈，完全不改动、不重启 IPv4 链路，在线业务扩容IPv6首选

### 3. 全自动配置清理机制
- Bond 模式：绑定完成自动清空所有 slave 从网卡三层IP，严格遵循 bonding 二层转发规范
- 单网卡模式：自动销毁残留 bond0 接口、清空bond所有IP；若SSH运行在bond0则跳过清理并给出手动操作提示
- 自动清理服务器所有其他物理网卡冗余IP，仅保留当前业务网卡地址

### 4. 完整中英双语国际化支持
- 全量提示、日志、告警、校验文本托管i18n，无硬编码中英文混杂
- 主菜单一键切换中文/英文界面，语言状态全局持久生效

### 5. 多层配置安全保障
- 修改 `/etc/network/interfaces`、`resolv.conf` 前自动生成带时间戳备份文件，故障可一键回滚
- 写入配置后执行 `ifup --no-act` 语法预校验，语法异常支持终止配置或忽略警告继续
- 自动停止冲突网络服务：NetworkManager / systemd-networkd，避免网络栈抢占冲突
- 自动检测并安装缺失依赖：iproute2 / ifupdown / ifenslave

### 6. 内置运维工具箱
- DNS连通测速：阿里云公共IPv4/IPv6 DNS，输出连通状态、平均延迟
- 开机自启配置管理：bond内核模块自动写入 `/etc/modules-load.d`，切换单网卡自动删除
- 支持3种工业级Bond模式：balance-rr轮询、active-backup主备、802.3ad LACP交换机聚合

### 7. Git 版本自动化管理
- 编译脚本自动读取Git标签注入版本号，二进制内置版本信息
- push.sh 一键脚本：自动提交代码、自增patch版本号、打Git标签、推送远程仓库、自动编译压缩二进制
- ftp.sh 一键上传编译好的netcfg二进制至远程FTP服务器，机房批量部署便捷

### 8. 一键系统初始化
- 自动识别Debian版本代号，替换APT源为华为云国内镜像
- 批量安装运维必备依赖：wget/curl/sudo/ifenslave
- 标准化加固SSH配置：仅密钥登录、禁用密码、限制登录尝试、关闭危险转发
- 初始化完成写入标记文件，避免DNS测速重复执行初始化操作

## 系统适配范围
### 支持系统
Debian 11(bullseye) / Debian 12(bookworm) / Debian 13(trixie)；使用传统 `/etc/network/interfaces` ifupdown 网络栈的Ubuntu系统
### 不兼容环境
Netplan、NetworkManager 作为主网络栈的系统（如新版Ubuntu桌面）
### 运行权限
必须 root / sudo 权限执行
### 运行依赖
编译静态无CGO二进制，服务器无需预装Go环境，仅依赖系统基础命令集

## 项目目录结构
```
debian-netcfg/
├── common.go        # 公共工具、日志颜色、IP校验、SSH路由保护、系统工具函数
├── network.go       # 网卡枚举、IP热加载、Bond清理、DNS写入、interfaces配置生成
├── bond.go          # Bond链路聚合完整交互与实时生效逻辑
├── single.go        # 单网卡DHCP/静态IP配置交互逻辑
├── ipv6.go          # 独立IPv6配置模块，不改动现有IPv4
├── dnstest.go       # DNS连通性测速工具，附带自动初始化触发逻辑
├── main.go          # 程序入口、主菜单、网卡状态总览展示
├── i18n.go          # 中英双语国际化词条字典
├── init.go          # 系统初始化：APT源替换、SSH加固、依赖安装
├── build.sh         # 静态编译脚本，自动注入Git版本号+UPX最高压缩
├── push.sh          # 一键发布脚本：提交代码、自增版本标签、推送、编译
├── ftp.sh           # FTP一键上传二进制到远程服务器
├── go.mod           # Go模块依赖定义
└── README.md       # 项目说明文档
```

## 编译方式
### 1. 本地手动编译（推荐开发调试）
```bash
# 自动读取Git标签注入版本号，UPX压缩最小体积
./build.sh

# 无Git环境简易编译（版本默认dev）
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o netcfg
chmod +x ./netcfg
```

### 2. 一键推送+自动编译（开发迭代）
```bash
bash push.sh
```
执行逻辑：
1. git add . 暂存全部修改，自动排除ftp.sh提交
2. 输入自定义commit备注，空输入默认「日常更新」
3. 读取最新Git标签，patch号自动+1并创建新tag
4. 拉取远程main分支rebase避免冲突，推送代码与标签
5. 自动调用build.sh编译压缩二进制netcfg

## 部署使用流程
### 1. 服务器上传并安装工具
```bash
# 赋予执行权限
chmod +x ./netcfg
# 移动到系统全局命令目录
mv ./netcfg /usr/local/bin/
```

### 2. 运行工具（必须root）
```bash
# root直接执行
./netcfg
# 普通用户sudo提权
sudo netcfg
```

### 3. 批量远程快速部署
本地编译完成后执行FTP上传脚本：
```bash
bash ftp.sh
```

## 主菜单功能完整说明
启动后自动展示所有物理网卡、bond0状态、当前IPv4/IPv6网关，菜单选项如下：
1. **Single NIC IP Configuration 单网卡IP配置**
    - 自动枚举过滤虚拟网卡（lo/docker/veth/tun/br等），仅展示物理网卡
    - 自动识别网卡当前工作模式：DHCP/静态IP/无地址
    - DHCP模式可选切换静态或保留自动获取；静态模式可复用现有参数或重配
    - IPv4同时支持CIDR数字(24)与点分掩码(255.255.255.0)自动转换
    - 可选同步配置静态IPv6地址+网关
    - 写入配置前自动备份interfaces，热加载IP不中断SSH
    - 自动清理其他网卡IP、销毁残留bond0（SSH不在bond0时）
    - 写入阿里云公共DNS，备份resolv.conf；最终输出完整网络校验信息

2. **NIC Bonding / Link Aggregation 网卡链路聚合**
    - 支持多物理网卡绑定，3种工作模式可选：
      - balance-rr：轮询负载均衡，流量均分多网卡
      - active-backup：主备故障转移，单链路工作，故障自动切换
      - 802.3ad LACP：交换机聚合，需交换机开启LACP协商，带宽叠加
    - 自动加载bonding内核模块，写入开机自启配置 `/etc/modules-load.d/bonding.conf`
    - 所有slave网卡自动设为manual二层模式，绑定完成清空从网卡三层IP
    - 支持同步配置静态IPv6地址
    - 实时ip命令创建bond0并配置miimon、lacp速率、哈希策略，无需重启网络
    - 配置完成自动ping网关校验连通性，输出bond运行模式

3. **Standalone IPv6 Config 单独配置IPv6**
    - 专为在线生产业务设计，**完全不修改、不重启IPv4**链路
    - 仅追加IPv6静态配置到现有网卡interfaces文件
    - 使用ip命令在线绑定IPv6地址与路由，无ifdown/ifup断网操作
    - 不会清理、改动任何IPv4地址与默认路由，业务扩容IPv6专用

4. **Network Connectivity Test DNS连通性测试**
    - 自动测速阿里云公共DNS：
      IPv4: 223.5.5.5 / 223.6.6.6
      IPv6: 2400:3200::1 / 2400:3200:baba::1
    - 输出每个DNS连通状态、平均往返延迟
    - 首次IPv4网络正常且未初始化系统时，自动触发系统初始化（仅执行一次）

5. **Initialize System 系统初始化**
    - 自动识别Debian版本，替换APT源为华为云国内镜像
    - 执行apt update并批量安装wget/curl/sudo/ifenslave
    - SSH安全加固：写入预设RSA公钥、禁用密码登录、限制登录参数、关闭危险转发
    - 校验sshd配置语法，自动适配多版本ssh服务重启命令
    - 初始化完成写入 `/etc/netcfg.initialized` 标记，防止重复执行

6. **Switch Language 切换语言**
    一键切换程序全局中英文界面，即时生效无需重启工具

7. **Update Script 在线更新工具自身**
    - 从远程地址下载最新netcfg二进制，自动覆盖 `/usr/local/bin/netcfg`
    - 优先使用wget下载，失败自动切换curl
    - 替换完成自动重启新版程序，无需手动执行

0. **Exit 退出工具**

## 核心SSH防断连安全机制
1. **SSH会话识别逻辑**
读取系统环境变量 `SSH_CONNECTION` 获取客户端IP，查询路由表识别当前业务出口网卡，清理其他网卡IP时自动跳过该网卡。

2. **IPv4平滑切换流程（无断网）**
1) 将新IP附加到网卡，新旧IP双地址共存
2) 添加新网关高优先级默认路由
3) 删除旧默认网关路由，提升新路由优先级
4) 延迟1秒等待网络稳定，删除旧业务IP

3. **接口双向清理规则**
- 切换Bond模式：清空所有slave物理网卡三层IP，仅bond0保留业务地址
- 切换单网卡模式：清空其他物理网卡IP；若SSH运行在bond0则跳过删除bond0并提示手动清理

4. **降级兜底策略**
所有配置优先使用ip命令热生效；热加载失败才降级执行ifdown/ifup重启网卡（存在短暂断连风险）

## 系统配置文件说明
1. `/etc/network/interfaces`
程序自动生成标准化永久网络配置，区分lo、单网卡、bond、IPv4/IPv6静态区块；每次写入自动生成时间戳备份 `interfaces.bak_20260806_153022`。

2. `/etc/resolv.conf`
默认写入阿里云公共DNS，兼容systemd-resolved resolvectl配置与直接写入两种模式；修改前生成备份文件。

3. `/etc/modules-load.d/bonding.conf`
Bond模式自动创建，写入bonding实现开机加载内核模块；切换回单网卡时自动删除。

4. `/etc/netcfg.initialized`
空标记文件，记录系统初始化完成状态，防止DNS测速模块重复执行初始化脚本。

## 版本号管理规则
1. 编译时通过 `-ldflags "-X main.Version=$VERSION"` 将Git标签注入二进制
2. 版本取值逻辑：
   - 存在Git标签：v主版本.次版本.patch 例 v1.0.2
   - 标签后存在新提交：v1.0.2-3-gxxxxxxx
   - 本地存在未提交修改：v1.0.2-dirty
3. push.sh自动递增patch号、创建Git标签，编译二进制版本与标签完全同步

## 常见问题FAQ
### Q：运行提示权限不足？
A：工具必须使用root用户或sudo提权执行。

### Q：系统使用Netplan/NetworkManager无法使用工具？
A：本工具基于传统ifupdown网络栈，需切换为 `/etc/network/interfaces` 管理网络。

### Q：切换Bond/单网卡时SSH会不会断开？
A：绝大多数在线操作使用ip热加载不会断连；仅当当前SSH运行在bond0，程序自动跳过bond0删除，需手动切物理网卡SSH后清理bond0。

### Q：服务器重启后网络配置丢失？
A：配置永久写入 `/etc/network/interfaces`，正常重启保留；若NetworkManager自动覆盖需执行初始化脚本关闭冲突服务。

### Q：配置IPv6提示网关添加警告？
A：内网无IPv6网关属于正常场景，仅告警不阻断配置，配置文件已永久保存不影响IPv4业务。

### Q：如何查看当前工具版本？
A：直接执行 `netcfg`，主菜单标题第一行展示内置版本号。

### Q：在线更新工具后版本号丢失？
A：版本号固化编译在二进制内部，远程更新包自带对应Git标签版本，更新后正常显示新版本。

## 开源生产使用说明
- 纯静态Go单二进制，无第三方运行依赖，可直接拷贝至任意Debian机房服务器部署
- 所有网络修改前置自动备份配置文件，配置异常可手动回滚bak备份
- 专为批量机房远程运维设计，规避传统改IP、绑网卡导致远程服务器失联风险
- 代码逻辑统一规整，模块拆分清晰，便于二次开发扩展新功能

## 致谢
1. 华为云 Debian APT 国内镜像源
2. 阿里云公共 IPv4/IPv6 DNS 解析服务
3. Debian 社区 ifupdown 传统网络栈
4. 所有测试、反馈工具的运维同仁

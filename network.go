package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const interfacesPath = "/etc/network/interfaces"
const bondModulePath = "/etc/modules-load.d/bonding.conf"

// 虚拟/容器接口前缀（ListPhysicalInterfaces 使用）
var virtualIfacePrefixes = []string{
	"lo", "docker", "veth", "virbr", "vmbr", "br-", "tap", "tun",
	"bond", "ifb", "dummy", "wg", "zt", "nlmon", "ip6tnl", "macvtap", "vnet",
}

// 独立 IPv6 配置时允许选择的接口前缀（保留 bond）
var ipv6SelectablePrefixes = []string{
	"lo", "docker", "veth", "virbr", "vmbr", "br-", "tap", "tun",
}

// isVirtualIface 以“前缀”而非“子串”判定，避免误伤名字中偶然含 lo/tun 的物理网卡
func isVirtualIface(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func filterInterfaces(out string, prefixes []string) []string {
	var ifaces []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		if name == "" || isVirtualIface(name, prefixes) {
			continue
		}
		ifaces = append(ifaces, name)
	}
	return ifaces
}

// ListPhysicalInterfaces 返回可用于业务配置的物理网卡（排除虚拟与 bond）
func ListPhysicalInterfaces() ([]string, error) {
	out, err := RunCmd("ip", "-br", "link", "show")
	if err != nil {
		return nil, fmt.Errorf(T("list_nic_fail"))
	}
	return filterInterfaces(out, virtualIfacePrefixes), nil
}

// ListAllInterfaces 返回所有可选接口（保留 bond，用于独立 IPv6 配置）
func ListAllInterfaces() ([]string, error) {
	out, err := RunCmd("ip", "-br", "link", "show")
	if err != nil {
		return nil, fmt.Errorf(T("list_nic_fail"))
	}
	return filterInterfaces(out, ipv6SelectablePrefixes), nil
}

// GetInterfaceStatus 判定接口是否可用。
// 同时接受 state UP 与 LOWER_UP 标志，避免 operstate=UNKNOWN 的设备被误判为 DOWN。
func GetInterfaceStatus(iface string) string {
	out, err := RunCmd("ip", "-o", "link", "show", "dev", iface)
	if err != nil {
		return "UNKNOWN"
	}
	if strings.Contains(out, "state UP") || strings.Contains(out, "LOWER_UP") {
		return "UP"
	}
	return "DOWN"
}

func GetInterfaceIPv4CIDR(iface string) string {
	out, err := RunCmd("ip", "-4", "addr", "show", "dev", iface)
	if err != nil {
		return ""
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "inet" && i+1 < len(fields) {
				return fields[i+1]
			}
		}
	}
	return ""
}

func GetInterfaceIPv6Global(iface string) string {
	out, err := RunCmd("ip", "-6", "addr", "show", "dev", iface)
	if err != nil {
		return "N/A"
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "inet6") && strings.Contains(line, "global") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "inet6" && i+1 < len(fields) {
					return fields[i+1]
				}
			}
		}
	}
	return "N/A"
}

func GetDefaultIPv6Gateway() string {
	out, err := RunCmd("ip", "-6", "route", "show", "default")
	if err != nil {
		return "N/A"
	}
	fields := strings.Fields(out)
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return "N/A"
}

// ipModeFromInterfacesFile 从配置文件解析某网卡的 IPv4 模式。
// 逐行解析并跳过注释，避免 `# iface eth0 inet dhcp` 这类注释被误判为真实配置。
func ipModeFromInterfacesFile(path, iface string) (mode string, useStatic bool) {
	useStatic = true
	data, err := os.ReadFile(path)
	if err != nil {
		return "", true
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 4 || fields[0] != "iface" || fields[1] != iface || fields[2] != "inet" {
			continue
		}
		switch fields[3] {
		case "dhcp":
			return "dhcp", false
		case "static":
			return "static", true
		}
	}
	return "", true
}

func DetectInterfaceIPMode(iface string) (mode string, useStatic bool, currentIP, currentMask, currentGW string) {
	mode, useStatic = ipModeFromInterfacesFile(interfacesPath, iface)
	switch mode {
	case "dhcp":
		Info(T("current_dhcp"))
	case "static":
		Info(T("current_static"))
	}

	if mode == "" {
		currentIPCIDR := GetInterfaceIPv4CIDR(iface)
		if currentIPCIDR != "" {
			currentIP = strings.Split(currentIPCIDR, "/")[0]
			if IsDHCPClient(currentIP) {
				mode = "dhcp"
				useStatic = false
				Info(T("current_dhcp"))
			} else {
				mode = "static"
				useStatic = true
				Info(T("current_static"))
			}
		} else {
			mode = "none"
			useStatic = true
			Info(T("no_active_ipv4"))
		}
	}

	ipCIDR := GetInterfaceIPv4CIDR(iface)
	if ipCIDR != "" {
		parts := strings.Split(ipCIDR, "/")
		currentIP = parts[0]
		cidr := 24
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &cidr)
		}
		currentMask = CIDRToNetmask(cidr)
		currentGW = GetDefaultGateway()
	}
	return
}

// ------------------------------
// 默认路由处理
// ------------------------------

type defaultRoute struct {
	via  string
	dev  string
	spec []string
}

// listDefaultRoutes 逐条解析默认路由，避免只处理单条 via oldGW
func listDefaultRoutes() []defaultRoute {
	out, err := RunCmd("ip", "route", "show", "default")
	if err != nil {
		return nil
	}
	var routes []defaultRoute
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		r := defaultRoute{spec: fields}
		for i, f := range fields {
			if f == "via" && i+1 < len(fields) {
				r.via = fields[i+1]
			}
			if f == "dev" && i+1 < len(fields) {
				r.dev = fields[i+1]
			}
		}
		routes = append(routes, r)
	}
	return routes
}

func deleteDefaultRoute(r defaultRoute) {
	if len(r.spec) == 0 {
		return
	}
	_ = RunCmdSilent("ip", append([]string{"route", "del"}, r.spec...)...)
}

// clearIPConflictOnOtherIfaces 若 newIP 已存在于其它接口，则从那些接口移除，
// 避免"同一 IP 出现在多块网卡"导致的 ARP 冲突与流量异常。
func clearIPConflictOnOtherIfaces(iface, newIP, newCIDR string) {
	allOut, _ := RunCmd("ip", "-4", "addr", "show")
	if !strings.Contains(allOut, newIP+"/") {
		return
	}
	Info(fmt.Sprintf(T("ip_conflict_detected"), newIP))
	for _, line := range strings.Split(allOut, "\n") {
		if !strings.Contains(line, "inet "+newIP+"/") {
			continue
		}
		parts := strings.Fields(line)
		for i, f := range parts {
			if f != "dev" || i+1 >= len(parts) {
				continue
			}
			conflictIface := parts[i+1]
			if conflictIface == iface {
				continue
			}
			Info(fmt.Sprintf(T("ip_conflict_cleaning"), conflictIface, newIP))
			// 用实际存在的地址规格删除，避免前缀不一致时删除失败
			spec := newCIDR
			for j, g := range parts {
				if g == "inet" && j+1 < len(parts) && strings.HasPrefix(parts[j+1], newIP+"/") {
					spec = parts[j+1]
				}
			}
			_ = RunCmdSilent("ip", "addr", "del", spec, "dev", conflictIface)
		}
	}
}

// ApplyIPv4Online 在线热应用 IPv4（尽量不中断 SSH）
func ApplyIPv4Online(iface string, newIP, newMask, newGW string, oldIP string) error {
	Info(T("apply_ipv4_online"))

	if GetInterfaceStatus(iface) != "UP" {
		Info(fmt.Sprintf(T("nic_down_enabling"), iface))
		err := RunCmdSilent("ip", "link", "set", iface, "up")
		if err != nil {
			return fmt.Errorf("failed to bring up interface %s: %v", iface, err)
		}
		Sleep(2)
		if GetInterfaceStatus(iface) != "UP" {
			Warn(fmt.Sprintf(T("nic_still_down"), iface))
		} else {
			Success(fmt.Sprintf(T("nic_link_up"), iface))
		}
	}

	mask := net.IPMask(net.ParseIP(newMask).To4())
	if mask == nil {
		return fmt.Errorf("invalid netmask: %s", newMask)
	}
	prefixLen, _ := mask.Size()
	newCIDR := fmt.Sprintf("%s/%d", newIP, prefixLen)

	// 网关网段预检：仅告警，不阻断流程
	if newGW != "" && newGW != "0.0.0.0" && ValidateIPv4(newGW) && !IsIPInCIDR(newGW, newCIDR) {
		Warn(fmt.Sprintf(T("gw_out_of_subnet"), newGW, newCIDR))
	}

	// 检测并清除其它接口上的相同 IP（防止 IP 冲突）
	clearIPConflictOnOtherIfaces(iface, newIP, newCIDR)

	sshPeerIP := GetCurrentSSHPeerIP()
	oldGW := GetDefaultGateway()
	sshDev := GetRouteDevForIP(sshPeerIP)

	if sshPeerIP != "" && sshDev != "" && sshDev != iface {
		Info(T("ssh_route_protection"))
		_ = RunCmdSilent("ip", "route", "add", sshPeerIP+"/32", "via", oldGW, "dev", sshDev)
	}

	// 用 replace 而不是 add：重复下发完全相同的地址时 add 会返回
	// "RTNETLINK answers: File exists"，进而触发 single.go 的 ifdown/ifup 回退（有断连风险）
	err := RunCmdSilent("ip", "addr", "replace", newCIDR, "dev", iface)
	if err != nil {
		return fmt.Errorf("failed to add new IP: %v", err)
	}
	Success(fmt.Sprintf(T("new_ip_bound"), newIP, iface))

	// ---- 默认路由：按 (via, dev) 判定，并逐条清理陈旧默认路由 ----
	if newGW != "" && newGW != "0.0.0.0" {
		current := listDefaultRoutes()
		desiredExists := false
		for _, r := range current {
			if r.via == newGW && r.dev == iface {
				desiredExists = true
			}
		}

		if desiredExists {
			// 目标默认路由已就位，仅清理其它陈旧条目
			for _, r := range current {
				if r.via == newGW && r.dev == iface {
					continue
				}
				deleteDefaultRoute(r)
			}
		} else if err := RunCmdSilent("ip", "route", "add", "default", "via", newGW, "dev", iface, "metric", "100"); err != nil {
			Warn(T("new_gw_fail"))
		} else {
			// 先加后删，再提升优先级，避免网络真空期
			for _, r := range current {
				if r.via == newGW && r.dev == iface {
					continue
				}
				deleteDefaultRoute(r)
			}
			if err := RunCmdSilent("ip", "route", "change", "default", "via", newGW, "dev", iface, "metric", "0"); err != nil {
				_ = RunCmdSilent("ip", "route", "replace", "default", "via", newGW, "dev", iface, "metric", "0")
				_ = RunCmdSilent("ip", "route", "del", "default", "via", newGW, "dev", iface, "metric", "100")
			}
			Success(fmt.Sprintf(T("gw_applied"), newGW))
		}
	}

	if oldIP != "" && oldIP != newIP {
		Sleep(1)
		// 显式带前缀删除：iproute2 对不带前缀的 addr del 只是"通配删除"兼容行为，
		// 官方已提示该行为未来会移除
		oldSpec := oldIP
		if out, err := RunCmd("ip", "-4", "-o", "addr", "show", "dev", iface); err == nil {
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Fields(line)
				for i, f := range fields {
					if f == "inet" && i+1 < len(fields) && strings.HasPrefix(fields[i+1], oldIP+"/") {
						oldSpec = fields[i+1]
					}
				}
			}
		}
		if err := RunCmdSilent("ip", "addr", "del", oldSpec, "dev", iface); err == nil {
			Info(fmt.Sprintf(T("old_ip_removed"), oldIP))
		} else {
			Warn(fmt.Sprintf(T("old_ip_remove_fail"), oldIP))
		}
	}

	return nil
}

func CleanBondResidual() {
	Info(T("clean_bond"))
	if RunCmdSilent("ip", "link", "show", "bond0") == nil {
		_ = RunCmdSilent("ip", "link", "set", "bond0", "down")
		_ = RunCmdSilent("ip", "link", "delete", "bond0")
		Sleep(1)
		Success(T("bond_removed"))
	}
	if _, err := os.Stat(bondModulePath); err == nil {
		_ = os.Remove(bondModulePath)
		Success(T("bond_module_removed"))
	}
	if _, err := os.Stat("/proc/net/bonding"); err == nil {
		entries, _ := os.ReadDir("/proc/net/bonding")
		if len(entries) == 0 {
			_ = RunCmdSilent("modprobe", "-r", "bonding")
		}
	}
}

func CleanOtherInterfaces(keepIface string) {
	Info(T("clean_other_nics"))
	sshPeerIP := GetCurrentSSHPeerIP()
	sshDev := GetRouteDevForIP(sshPeerIP)
	skipped := ""

	allNics, err := ListPhysicalInterfaces()
	if err != nil {
		Warn(err.Error())
		return
	}

	var targets []string
	for _, nic := range allNics {
		if nic == keepIface {
			continue
		}
		if nic == sshDev {
			skipped = nic
			continue
		}
		targets = append(targets, nic)
	}

	// 显式列出将被清理的网卡，避免用户对破坏范围无感
	if len(targets) > 0 {
		Info(fmt.Sprintf(T("clean_nic_targets"), strings.Join(targets, " ")))
	}
	for _, nic := range targets {
		_ = RunCmdSilent("ip", "addr", "flush", "dev", nic)
		_ = RunCmdSilent("ip", "-6", "addr", "flush", "dev", nic)
	}

	if skipped != "" {
		Warn(fmt.Sprintf(T("ssh_nic_skipped"), skipped))
		Warn(fmt.Sprintf(T("ssh_nic_manual_tip"), skipped))
	} else {
		Success(T("other_nics_cleared"))
	}
}

// mergeResolvContent 生成 resolv.conf 内容：阿里云 DNS 优先，
// 原有其它 nameserver 追加保留，search/domain/options/sortlist 等行原样保留。
func mergeResolvContent(existing string, enableIPv6 bool) (content string, preserved []string) {
	seen := map[string]bool{}
	var extra []string

	add := func(ip string) {
		if ip == "" || seen[ip] {
			return
		}
		seen[ip] = true
		content += "nameserver " + ip + "\n"
	}

	for _, dns := range AliDNS4 {
		add(dns)
	}
	if enableIPv6 {
		for _, dns := range AliDNS6 {
			add(dns)
		}
	}

	for _, line := range strings.Split(existing, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			if !seen[fields[1]] {
				preserved = append(preserved, fields[1])
				add(fields[1])
			}
			continue
		}
		// search / domain / options / sortlist 等指令不能被丢掉，
		// 否则内网短名解析与 ndots/timeout 策略会被重置
		extra = append(extra, trimmed)
	}
	for _, l := range extra {
		content += l + "\n"
	}
	return content, preserved
}

// buildResolvContent 读取现有 resolv.conf 并合并出新内容
func buildResolvContent(enableIPv6 bool) string {
	var existing string
	if data, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		existing = string(data)
	}
	content, preserved := mergeResolvContent(existing, enableIPv6)
	if len(preserved) > 0 {
		Info(fmt.Sprintf(T("dns_existing_kept"), strings.Join(preserved, " ")))
	}
	return content
}

func ConfigureDNS(targetIface string, enableIPv6 bool) {
	Info(T("config_dns"))
	const resolvPath = "/etc/resolv.conf"

	resolvBackup := fmt.Sprintf("%s.bak_%s", resolvPath, time.Now().Format("20060102_150405"))
	if data, err := os.ReadFile(resolvPath); err == nil {
		_ = os.WriteFile(resolvBackup, data, 0644)
		Success(fmt.Sprintf(T("backup_dns"), resolvBackup))
	}

	// 记录 immutable 属性，写入后恢复原状
	immutable := false
	if out, err := RunCmd("lsattr", "-d", resolvPath); err == nil {
		if fields := strings.Fields(out); len(fields) > 0 && strings.Contains(fields[0], "i") {
			immutable = true
		}
	}
	if immutable {
		_ = RunCmdSilent("chattr", "-i", resolvPath)
	}

	// 优先尝试 resolvectl，并校验是否真正生效
	applied := false
	if CommandExists("resolvectl") {
		args := append([]string{"dns", targetIface}, AliDNS4...)
		if RunCmdSilent("resolvectl", args...) == nil {
			if enableIPv6 {
				args6 := append([]string{"dns", targetIface}, AliDNS6...)
				_ = RunCmdSilent("resolvectl", args6...)
			}
			if out, err := RunCmd("resolvectl", "dns", targetIface); err == nil && strings.Contains(out, AliDNS4[0]) {
				applied = true
				Success(T("dns_via_resolvectl"))
			}
		}
	}

	if !applied {
		if err := os.WriteFile(resolvPath, []byte(buildResolvContent(enableIPv6)), 0644); err != nil {
			Error(fmt.Sprintf(T("write_fail"), err))
		} else {
			Success(T("dns_written"))
		}
	}

	if immutable {
		_ = RunCmdSilent("chattr", "+i", resolvPath)
	}
}

func ValidateConfig(iface string) bool {
	Info(T("validate_config"))
	out, err := RunCmd("ifup", "--no-act", iface)
	if err != nil {
		Warn(fmt.Sprintf(T("syntax_warn"), strings.TrimSpace(out)))
		if !ReadConfirm(T("continue_anyway"), true) {
			Error(T("abort_config"))
			return false
		}
	}
	return true
}

// ApplyIPv6Online 在线应用 IPv6。
// 只增删本工具管理的地址，不再 flush 全部 global 地址（保留 SLAAC/DHCPv6 地址）。
func ApplyIPv6Online(iface, ipv6Addr, ipv6GW, oldAddr string) error {
	Info(T("apply_ipv6_online"))

	if GetInterfaceStatus(iface) != "UP" {
		_ = RunCmdSilent("ip", "link", "set", iface, "up")
		Sleep(1)
	}

	if oldAddr != "" && oldAddr != ipv6Addr {
		if RunCmdSilent("ip", "-6", "addr", "del", oldAddr, "dev", iface) == nil {
			Info(fmt.Sprintf(T("ipv6_old_removed"), oldAddr))
		}
	}

	if err := RunCmdSilent("ip", "-6", "addr", "replace", ipv6Addr, "dev", iface); err != nil {
		Error(T("ipv6_addr_fail"))
		return err
	}
	Success(fmt.Sprintf(T("ipv6_addr_added"), ipv6Addr, iface))

	if ipv6GW != "" {
		if err := RunCmdSilent("ip", "-6", "route", "replace", "default", "via", ipv6GW, "dev", iface); err != nil {
			Warn(T("ipv6_gw_fail"))
			Warn(T("ipv6_gw_fail_tip"))
		} else {
			Success(fmt.Sprintf(T("ipv6_gw_added"), ipv6GW))
		}
	}
	return nil
}

// ------------------------------
// interfaces 文件块级写入
// ------------------------------

type ifaceBlock struct {
	iface string
	lines []string
}

func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

// normalizeBlankLines 压缩连续空行并去掉首尾空行
func normalizeBlankLines(lines []string) []string {
	var out []string
	blank := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			if blank || len(out) == 0 {
				continue
			}
			blank = true
			out = append(out, "")
			continue
		}
		blank = false
		out = append(out, l)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// rewriteInterfaces 只替换被管理网卡的 stanza 与 auto/allow-hotplug 行，
// 保留 lo、source 指令以及其它未管理网卡的配置，不再整体覆盖文件。
func rewriteInterfaces(path string, blocks []ifaceBlock) error {
	managed := make(map[string]bool, len(blocks))
	for _, b := range blocks {
		managed[b.iface] = true
	}

	var existing []string
	if data, err := os.ReadFile(path); err == nil {
		existing = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return err
	}

	var kept []string
	for i := 0; i < len(existing); i++ {
		line := existing[i]
		trimmed := strings.TrimSpace(line)

		// 清理旧版本写入的自动生成标记
		if strings.HasPrefix(trimmed, "# Auto generated") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && (fields[0] == "auto" || fields[0] == "allow-hotplug") {
			var remain []string
			for _, name := range fields[1:] {
				if !managed[name] {
					remain = append(remain, name)
				}
			}
			if len(remain) == 0 {
				continue
			}
			kept = append(kept, fields[0]+" "+strings.Join(remain, " "))
			continue
		}
		if len(fields) >= 2 && fields[0] == "iface" && managed[fields[1]] {
			// 跳过整个 stanza（含后续缩进行）
			for i+1 < len(existing) && isIndented(existing[i+1]) {
				i++
			}
			continue
		}
		kept = append(kept, line)
	}

	kept = normalizeBlankLines(kept)

	// 确保环回口配置存在
	hasLoopback := false
	for _, l := range kept {
		if strings.HasPrefix(strings.TrimSpace(l), "iface lo ") {
			hasLoopback = true
			break
		}
	}
	if !hasLoopback {
		kept = append([]string{"auto lo", "iface lo inet loopback", ""}, kept...)
	}

	for _, b := range blocks {
		kept = append(kept, "")
		kept = append(kept, b.lines...)
	}

	kept = normalizeBlankLines(kept)
	final := strings.Join(kept, "\n") + "\n"

	if err := os.WriteFile(path, []byte(final), 0644); err != nil {
		return err
	}
	_ = os.Chmod(path, 0644)
	Info(T("interfaces_preserved"))
	return nil
}

// upsertIPv6Block 追加/替换指定网卡的 inet6 stanza，保留文件其余内容
func upsertIPv6Block(path, iface, ipv6Addr, ipv6GW string) error {
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return err
	}

	// 1. 移除旧的 inet6 stanza
	var out []string
	for i := 0; i < len(lines); i++ {
		fields := strings.Fields(strings.TrimSpace(lines[i]))
		if len(fields) >= 3 && fields[0] == "iface" && fields[1] == iface && strings.HasPrefix(fields[2], "inet6") {
			for i+1 < len(lines) && isIndented(lines[i+1]) {
				i++
			}
			continue
		}
		out = append(out, lines[i])
	}

	// 2. 定位该网卡 inet/manual stanza 的起止行
	stanzaStart, insertIdx := -1, -1
	inBlock := false
	for i, line := range out {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && fields[0] == "iface" && fields[1] == iface && !strings.HasPrefix(fields[2], "inet6") {
			if stanzaStart == -1 {
				stanzaStart = i
			}
			inBlock = true
			continue
		}
		if inBlock {
			if isIndented(line) {
				insertIdx = i + 1
				continue
			}
			break
		}
	}

	block := []string{
		"",
		fmt.Sprintf("iface %s inet6 static", iface),
		"    address " + ipv6Addr,
		"    gateway " + ipv6GW,
	}

	// 3. 组装：需要时补 auto <iface>（否则重启后 ifup -a 不会拉起该配置）
	hasAuto := false
	for _, l := range out {
		f := strings.Fields(strings.TrimSpace(l))
		if len(f) >= 2 && (f[0] == "auto" || f[0] == "allow-hotplug") {
			for _, name := range f[1:] {
				if name == iface {
					hasAuto = true
				}
			}
		}
	}

	if insertIdx == -1 {
		// 该网卡完全没有 stanza：追加 auto + inet6
		if !hasAuto {
			out = append(out, "auto "+iface)
		}
		out = append(out, block...)
	} else {
		merged := make([]string, 0, len(out)+len(block)+1)
		merged = append(merged, out[:insertIdx]...)
		merged = append(merged, block...)
		merged = append(merged, out[insertIdx:]...)
		out = merged
		if !hasAuto {
			// 插到该网卡第一个 stanza 之前（auto 必须在 stanza 之前才直观）
			merged = make([]string, 0, len(out)+1)
			merged = append(merged, out[:stanzaStart]...)
			merged = append(merged, "auto "+iface)
			merged = append(merged, out[stanzaStart:]...)
			out = merged
		}
	}

	out = normalizeBlankLines(out)
	final := strings.Join(out, "\n") + "\n"

	if err := os.WriteFile(path, []byte(final), 0644); err != nil {
		return err
	}
	_ = os.Chmod(path, 0644)
	Info(T("interfaces_preserved"))
	Success(fmt.Sprintf(T("ipv6_config_updated"), iface))
	return nil
}

// getConfiguredIPv6Address 读取 interfaces 中该网卡已配置的 IPv6 地址
func getConfiguredIPv6Address(path, iface string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && fields[0] == "iface" && fields[1] == iface && strings.HasPrefix(fields[2], "inet6") {
			inBlock = true
			continue
		}
		if inBlock {
			if !isIndented(line) {
				break
			}
			if len(fields) >= 2 && fields[0] == "address" {
				return fields[1]
			}
		}
	}
	return ""
}

// GetConfiguredIPv6Address 供调用方在写配置前读取旧地址
func GetConfiguredIPv6Address(iface string) string {
	return getConfiguredIPv6Address(interfacesPath, iface)
}

// AddIPv6ToConfig 仅追加 IPv6，不修改 IPv4 stanza
func AddIPv6ToConfig(iface, ipv6Addr, ipv6GW string) error {
	return upsertIPv6Block(interfacesPath, iface, ipv6Addr, ipv6GW)
}

func WriteSingleConfig(iface string, useStatic bool, ip, netmask, gateway string, enableIPv6 bool, ipv6Addr, ipv6GW string) error {
	lines := []string{"auto " + iface}
	if !useStatic {
		lines = append(lines, fmt.Sprintf("iface %s inet dhcp", iface))
	} else {
		lines = append(lines,
			fmt.Sprintf("iface %s inet static", iface),
			"    address "+ip,
			"    netmask "+netmask,
			"    gateway "+gateway,
		)
	}
	if enableIPv6 {
		lines = append(lines,
			"",
			fmt.Sprintf("iface %s inet6 static", iface),
			"    address "+ipv6Addr,
			"    gateway "+ipv6GW,
		)
	}
	return rewriteInterfaces(interfacesPath, []ifaceBlock{{iface: iface, lines: lines}})
}

func WriteBondConfig(nics []string, ip, netmask, gateway string, mode string, enableIPv6 bool, ipv6Addr, ipv6GW string) error {
	blocks := make([]ifaceBlock, 0, len(nics)+1)
	for _, nic := range nics {
		blocks = append(blocks, ifaceBlock{
			iface: nic,
			lines: []string{
				"auto " + nic,
				fmt.Sprintf("iface %s inet manual", nic),
				"    bond-master bond0",
			},
		})
	}

	bondLines := []string{
		"auto bond0",
		"iface bond0 inet static",
		"    address " + ip,
		"    netmask " + netmask,
		"    gateway " + gateway,
		"    dns-nameservers " + strings.Join(AliDNS4, " "),
		"    bond-mode " + mode,
		"    bond-miimon 100",
		"    bond-slaves " + strings.Join(nics, " "),
	}
	if mode == "802.3ad" {
		bondLines = append(bondLines,
			"    bond-lacp-rate fast",
			"    bond-xmit-hash-policy layer3+4",
		)
	}
	if enableIPv6 {
		bondLines = append(bondLines,
			"",
			"iface bond0 inet6 static",
			"    address "+ipv6Addr,
			"    gateway "+ipv6GW,
		)
	}
	blocks = append(blocks, ifaceBlock{iface: "bond0", lines: bondLines})

	return rewriteInterfaces(interfacesPath, blocks)
}

package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

func BondConfig() bool {
	fmt.Println(T("bond_title"))
	fmt.Println()
	CheckRoot()
	if err := EnsureDependencies(); err != nil {
		Error(err.Error())
		return false
	}
	DisableConflictServices()
	if RunCmdSilent("modprobe", "-n", "bonding") != nil {
		Error(T("bond_module_missing"))
		return false
	}
	Info(T("detect_nics"))
	physNics, err := ListPhysicalInterfaces()
	if err != nil {
		Error(err.Error())
		return false
	}
	if len(physNics) == 0 {
		Error(T("nics_needed"))
		return false
	}
	fmt.Println(T("available_nics"))
	for i, nic := range physNics {
		fmt.Printf("  %d. %s\n", i+1, nic)
	}
	var selectedNics []string
	for {
		input := ReadInput(T("select_2_nics"), "")
		if InputClosed() || input == "0" {
			Info(T("cancelled"))
			return false
		}
		parts := strings.Fields(input)
		if len(parts) == 0 {
			Warn(T("enter_2_numbers"))
			continue
		}
		seen := make(map[int]bool)
		selectedNics = nil
		valid := true
		for _, p := range parts {
			var idx int
			if _, err := fmt.Sscanf(p, "%d", &idx); err != nil {
				Warn(T("must_be_numbers"))
				valid = false
				break
			}
			idx--
			if idx < 0 || idx >= len(physNics) {
				Warn(T("number_out_range"))
				valid = false
				break
			}
			if seen[idx] {
				Warn(T("same_nic_error"))
				continue
			}
			seen[idx] = true
			selectedNics = append(selectedNics, physNics[idx])
		}
		if !valid || len(selectedNics) == 0 {
			continue
		}
		break
	}
	Success(fmt.Sprintf(T("selected_bond_nics"), strings.Join(selectedNics, " ")))
	var currentIP, currentMask, currentGW string
	if RunCmdSilent("ip", "link", "show", "bond0") == nil {
		_, _, currentIP, currentMask, currentGW = DetectInterfaceIPMode("bond0")
	} else {
		defaultNic := selectedNics[0]
		ipCIDR := GetInterfaceIPv4CIDR(defaultNic)
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
	}
	Info(T("config_ipv4_bond"))
	ipv4Addr, ipv4Netmask, ipv4Gateway := PromptIPv4Config(currentIP, currentMask, currentGW)
	if InputClosed() {
		Info(T("cancelled"))
		return false
	}
	fmt.Println()
	fmt.Println(T("bond_modes_title"))
	fmt.Println(T("bond_mode1"))
	fmt.Println(T("bond_mode2"))
	fmt.Println(T("bond_mode3"))
	modeInput := ReadInput(T("select_bond_mode"), "3")
	if InputClosed() || modeInput == "0" {
		Info(T("cancelled"))
		return false
	}
	var bondMode string
	switch modeInput {
	case "2":
		bondMode = "active-backup"
	case "3":
		bondMode = "802.3ad"
	default:
		bondMode = "balance-rr"
	}
	Success(fmt.Sprintf(T("bond_mode_set"), bondMode))
	configIPv6 := false
	var ipv6Addr, ipv6Gateway string
	fmt.Println()
	Interact(T("ipv6_config") + "\n")
	if ReadConfirm(T("config_ipv6_now"), false) {
		configIPv6 = true
		ipv6Addr, ipv6Gateway = PromptIPv6Config()
		if InputClosed() {
			Info(T("cancelled"))
			return false
		}
	} else {
		Info(T("ipv6_skipped"))
	}

	if _, err := os.Stat("/proc/net/bonding"); os.IsNotExist(err) {
		if err := RunCmdSilent("modprobe", "bonding"); err != nil {
			Error(T("bond_module_fail"))
			return false
		}
		if _, err := os.Stat("/proc/net/bonding"); os.IsNotExist(err) {
			Error(T("bond_proc_unavail"))
			return false
		}
	}
	// 先清理旧 bond 残留。注意 CleanBondResidual 会删除 bonding.conf，
	// 因此必须放在写"开机加载模块"文件之前，否则刚写好的文件会被立刻删掉。
	CleanBondResidual()

	_ = os.MkdirAll("/etc/modules-load.d", 0755)
	_ = os.WriteFile(bondModulePath, []byte("bonding\n"), 0644)
	_ = RunCmdSilent("systemctl", "enable", "systemd-modules-load.service")
	_ = RunCmdSilent("systemctl", "enable", "networking.service")

	// 记录旧 IPv6 地址，供在线切换时精确清理（避免 flush 掉全部 global 地址）
	oldIPv6 := GetConfiguredIPv6Address("bond0")

	backup := BackupFile(interfacesPath)
	Info(T("write_bond_config"))
	if err := WriteBondConfig(selectedNics, ipv4Addr, ipv4Netmask, ipv4Gateway, bondMode, configIPv6, ipv6Addr, ipv6Gateway); err != nil {
		Error(fmt.Sprintf(T("write_fail"), err))
		RestoreFile(backup, interfacesPath)
		return false
	}
	if !ValidateConfig("bond0") {
		RestoreFile(backup, interfacesPath)
		return false
	}
	Info(T("apply_network"))

	// 创建 bond0（mode / miimon 在创建时确定）
	if err := RunCmdSilent("ip", "link", "add", "bond0", "type", "bond", "mode", bondMode, "miimon", "100"); err != nil {
		Error(fmt.Sprintf("failed to create bond0: %v", err))
		return false
	}

	// 关键：lacp_rate 必须在 bond0 处于 DOWN 时写入。
	// 内核会拒绝在接口 UP 之后修改 lacp_rate
	// ("unable to set option because the bond is up")，
	// Debian 的 /etc/network/if-pre-up.d/ifenslave 同样是在 up 之前用
	// sysfs_change_down 写入该值。xmit_hash_policy 虽然运行时可改，
	// 但在这里一并写入可保证两个参数一定生效。
	if bondMode == "802.3ad" {
		Info(T("apply_bond_params"))
		if err := RunCmdSilent("ip", "link", "set", "bond0", "type", "bond", "lacp_rate", "fast"); err != nil {
			Warn(fmt.Sprintf(T("bond_param_fail"), "lacp_rate=fast"))
		}
		if err := RunCmdSilent("ip", "link", "set", "bond0", "type", "bond", "xmit_hash_policy", "layer3+4"); err != nil {
			Warn(fmt.Sprintf(T("bond_param_fail"), "xmit_hash_policy=layer3+4"))
		}
	}

	// 计算 IPv4 前缀长度
	mask := net.IPMask(net.ParseIP(ipv4Netmask).To4())
	prefixLen, _ := mask.Size()
	bondCIDR := fmt.Sprintf("%s/%d", ipv4Addr, prefixLen)

	// 清除"未选中的其它网卡"上的同一 IP，否则会出现同 IP 多网卡冲突
	// （这正是从单网卡切到 bond 时最常见的故障原因）
	clearIPConflictOnOtherIfaces("bond0", ipv4Addr, bondCIDR)

	_ = RunCmdSilent("ip", "addr", "replace", bondCIDR, "dev", "bond0")

	// 挂载 slave 网卡
	for _, nic := range selectedNics {
		_ = RunCmdSilent("ip", "link", "set", nic, "down")
		_ = RunCmdSilent("ip", "addr", "flush", "dev", nic)
		_ = RunCmdSilent("ip", "link", "set", nic, "master", "bond0")
		_ = RunCmdSilent("ip", "link", "set", nic, "up")
	}

	// 启动 bond0，先 down 再 up 强制协商（触发 LACP 重新协商）
	_ = RunCmdSilent("ip", "link", "set", "bond0", "down")
	Sleep(1)
	_ = RunCmdSilent("ip", "link", "set", "bond0", "up")

	// 回读内核真实生效值（不信任退出码，避免"以为下发了其实没生效"）
	if bondMode == "802.3ad" {
		lacp := readBondOption("bond0", "lacp_rate")
		hash := readBondOption("bond0", "xmit_hash_policy")
		if lacp == "fast" && hash == "layer3+4" {
			Success(fmt.Sprintf(T("bond_params_applied"), lacp, hash))
		} else {
			Warn(fmt.Sprintf(T("bond_params_mismatch"), lacp, hash))
			Warn(T("bond_params_warn"))
		}
	}

	// 添加默认路由
	if ipv4Gateway != "" && ipv4Gateway != "0.0.0.0" {
		_ = RunCmdSilent("ip", "route", "replace", "default", "via", ipv4Gateway, "dev", "bond0")
	}

	// 在线应用 IPv6（原实现仅写入配置文件，未在线生效）
	if configIPv6 {
		if err := ApplyIPv6Online("bond0", ipv6Addr, ipv6Gateway, oldIPv6); err != nil {
			Warn(T("ipv6_gw_warn"))
		}
	}

	ConfigureDNS("bond0", configIPv6)
	Sleep(2)
	Info(T("clean_slave_nics"))
	for _, nic := range selectedNics {
		_ = RunCmdSilent("ip", "-4", "addr", "flush", "dev", nic)
		_ = RunCmdSilent("ip", "-6", "addr", "flush", "dev", nic, "scope", "global")
	}
	Success(T("slave_nics_cleared"))
	fmt.Println()
	Info(T("final_verify"))
	if out, err := RunCmd("ip", "link", "show", "bond0"); err == nil && (strings.Contains(out, "state UP") || strings.Contains(out, "LOWER_UP")) {
		Success(T("bond_active"))
		if modeOut, err := RunCmd("grep", "Bonding Mode", "/proc/net/bonding/bond0"); err == nil {
			modeStr := strings.TrimSpace(strings.SplitN(modeOut, ":", 2)[1])
			fmt.Printf(T("bond_mode_label")+"\n", modeStr)
		}
	} else {
		Error(T("bond_fail_start"))
	}
	fmt.Printf(T("verify_active_ipv4")+"\n", GetInterfaceIPv4CIDR("bond0"))
	fmt.Printf(T("verify_gw")+"\n", GetDefaultGateway())
	if configIPv6 {
		fmt.Printf(T("verify_active_ipv6")+"\n", GetInterfaceIPv6Global("bond0"))
	}
	Sleep(1)
	if ipv4Gateway != "" && ipv4Gateway != "0.0.0.0" {
		if RunCmdSilent("ping", "-c", "2", "-W", "2", ipv4Gateway) == nil {
			Success(T("gw_ping_ok"))
		} else {
			Warn(T("gw_ping_fail"))
		}
	}
	// ifenslave 2.13 起不再提供同名可执行文件，只提供 ifupdown hook 脚本，
	// 因此必须同时检测 hook 是否存在，否则会误判为"未安装"而反复 apt install。
	if !ifenslaveAvailable() {
		Info(T("ifenslave_missing_tip"))
		Info(T("try_install_ifenslave"))
		if installErr := RunCmdSilent("apt", "install", "-y", "-qq", "ifenslave"); installErr != nil {
			Warn(T("ifenslave_install_fail"))
			Warn(T("offline_deb_tip"))
		} else {
			Success(T("ifenslave_install_ok"))
		}
	}
	fmt.Println()
	Success(T("bond_complete"))
	return true
}

// ------------------------------
// Bond 参数回读与依赖检测
// ------------------------------

// ifenslaveHookPath Debian ifenslave 包提供的 ifupdown hook
// （bond-* 选项就是由它在上电/ifup 时写入内核的）
const ifenslaveHookPath = "/etc/network/if-pre-up.d/ifenslave"

// ifenslaveAvailable 判断 ifenslave 是否可用。
// 新版 Debian（2.13+）只有 hook 脚本、没有同名可执行文件，故两者都要检测。
func ifenslaveAvailable() bool {
	if CommandExists("ifenslave") {
		return true
	}
	if _, err := os.Stat(ifenslaveHookPath); err == nil {
		return true
	}
	return false
}

// parseBondOptionValue 解析 bonding sysfs 文件内容，取首个字段。
// 例如 "layer3+4 1\n" -> "layer3+4"，"fast 1\n" -> "fast"，空内容 -> "unknown"。
func parseBondOptionValue(content string) string {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return "unknown"
	}
	return fields[0]
}

// readBondOption 读取 /sys/class/net/<iface>/bonding/<opt> 的真实生效值
func readBondOption(iface, opt string) string {
	data, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/bonding/%s", iface, opt))
	if err != nil {
		return "unknown"
	}
	return parseBondOptionValue(string(data))
}

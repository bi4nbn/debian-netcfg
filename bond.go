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
	DisableConflictServices()
	// 唯一必需前置依赖：内核 bonding 模块
	if RunCmdSilent("modprobe", "-n", "bonding") != nil {
		Fatal(T("bond_module_missing"))
	}
	Info(T("detect_nics"))
	physNics := ListPhysicalInterfaces()
	if len(physNics) == 0 {
		Fatal(T("nics_needed"))
	}
	fmt.Println(T("available_nics"))
	for i, nic := range physNics {
		fmt.Printf("  %d. %s\n", i+1, nic)
	}
	var selectedNics []string
	for {
		input := ReadInput(T("select_2_nics"), "")
		if input == "0" {
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
			_, err := fmt.Sscanf(p, "%d", &idx)
			if err != nil {
				valid = false
				break
			}
			idx--
			if idx < 0 || idx >= len(physNics) {
				valid = false
				break
			}
			if seen[idx] {
				continue
			}
			seen[idx] = true
			selectedNics = append(selectedNics, physNics[idx])
		}
		if !valid || len(selectedNics) == 0 {
			Warn(T("must_be_numbers"))
			continue
		}
		break
	}
	Success(fmt.Sprintf(T("selected_bond_nics"), strings.Join(selectedNics, " ")))
	// 读取现有配置默认值
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
	fmt.Println()
	fmt.Println(T("bond_modes_title"))
	fmt.Println(T("bond_mode1"))
	fmt.Println(T("bond_mode2"))
	fmt.Println(T("bond_mode3"))
	modeInput := ReadInput(T("select_bond_mode"), "1")
	if modeInput == "0" {
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
	// IPv6 配置
	configIPv6 := false
	var ipv6Addr, ipv6Gateway string
	fmt.Println()
	Interact(T("ipv6_config") + "\n")
	if ReadConfirm(T("config_ipv6_now"), false) {
		configIPv6 = true
		ipv6Addr, ipv6Gateway = PromptIPv6Config()
	} else {
		Info(T("ipv6_skipped"))
	}
	// 加载 bonding 内核模块
	if _, err := os.Stat("/proc/net/bonding"); os.IsNotExist(err) {
		err := RunCmdSilent("modprobe", "bonding")
		if err != nil {
			Fatal(T("bond_module_fail"))
		}
	}
	// 写入开机模块加载配置
	_ = os.MkdirAll("/etc/modules-load.d", 0755)
	_ = os.WriteFile(bondModulePath, []byte("bonding\n"), 0644)
	_ = RunCmdSilent("systemctl", "enable", "systemd-modules-load.service")
	_ = RunCmdSilent("systemctl", "enable", "networking.service")
	// 清理旧 bond 残留
	CleanBondResidual()
	// 备份配置
	BackupFile(interfacesPath)
	// 写入永久配置文件（标准格式，兼容 ifenslave 环境）
	Info(T("write_bond_config"))
	err := WriteBondConfig(selectedNics, ipv4Addr, ipv4Netmask, ipv4Gateway, bondMode, configIPv6, ipv6Addr, ipv6Gateway)
	if err != nil {
		Fatal(fmt.Sprintf(T("write_fail"), err))
	}
	if !ValidateConfig("bond0") {
		return false
	}
	_ = os.Chmod(interfacesPath, 0644)
	// 实时生效：统一用 ip 命令，不依赖 ifenslave，保证无网也能配置成功
	Info(T("apply_network"))
	// 1. 创建 bond0 接口并设置模式
	_ = RunCmdSilent("ip", "link", "add", "bond0", "type", "bond", "mode", bondMode)
	// 2. 配置 IPv4 地址
	mask := net.IPMask(net.ParseIP(ipv4Netmask).To4())
	prefixLen, _ := mask.Size()
	_ = RunCmdSilent("ip", "addr", "add", fmt.Sprintf("%s/%d", ipv4Addr, prefixLen), "dev", "bond0")
	// 3. 物理网卡加入 bond
	for _, nic := range selectedNics {
		_ = RunCmdSilent("ip", "link", "set", nic, "down")
		_ = RunCmdSilent("ip", "addr", "flush", "dev", nic)
		_ = RunCmdSilent("ip", "link", "set", nic, "master", "bond0")
		_ = RunCmdSilent("ip", "link", "set", nic, "up")
	}
	// 4. 启动 bond0
	_ = RunCmdSilent("ip", "link", "set", "bond0", "up")
	// 5. 设置默认网关
	if ipv4Gateway != "" {
		_ = RunCmdSilent("ip", "route", "replace", "default", "via", ipv4Gateway, "dev", "bond0")
	}
	// 配置 DNS
	ConfigureDNS("bond0", configIPv6)
	Sleep(2)
	// 清理从网卡残留 IP
	Info(T("clean_slave_nics"))
	for _, nic := range selectedNics {
		_ = RunCmdSilent("ip", "-4", "addr", "flush", "dev", nic)
		_ = RunCmdSilent("ip", "-6", "addr", "flush", "dev", nic, "scope", "global")
	}
	Success(T("slave_nics_cleared"))
	// 最终校验
	fmt.Println()
	Info(T("final_verify"))
	if out, err := RunCmd("ip", "link", "show", "bond0"); err == nil && strings.Contains(out, "state UP") {
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
	if RunCmdSilent("ping", "-c", "2", "-W", "2", ipv4Gateway) == nil {
		Success(T("gw_ping_ok"))
	} else {
		Warn(T("gw_ping_fail"))
	}
	// ========== 后置静默安装 ifenslave ==========
	// 配置已生效、连通性测试完成后，后台尝试补装；有网自动装上，无网不影响现有配置
	if !CommandExists("ifenslave") {
		Info(T("try_install_ifenslave"))
		installErr := RunCmdSilent("apt", "install", "-y", "-qq", "ifenslave")
		if installErr != nil {
			Warn(T("ifenslave_install_fail"))
		} else {
			Success(T("ifenslave_install_ok"))
		}
	}
	fmt.Println()
	Success(T("bond_complete"))
	return true
}

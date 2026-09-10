package main

import (
	"fmt"
	"os"
	"strings"
)

func SingleNicConfig() bool {
	fmt.Println(T("single_title"))
	fmt.Println()
	CheckRoot()
	if err := EnsureDependencies(); err != nil {
		Error(err.Error())
		return false
	}
	// 单网卡流程同样需要停止冲突的网络管理服务
	DisableConflictServices()
	Info(T("detect_nics"))
	ifaces, err := ListPhysicalInterfaces()
	if err != nil {
		Error(err.Error())
		return false
	}
	if len(ifaces) == 0 {
		Error(T("no_valid_nics"))
		return false
	}
	Info(T("available_nics"))
	for i, iface := range ifaces {
		fmt.Printf("  %d. %s\n", i+1, iface)
	}
	indexStr := ReadInput(T("select_nic_prompt"), "1")
	if InputClosed() || indexStr == "0" {
		Info(T("cancelled"))
		return false
	}
	index := 1
	fmt.Sscanf(indexStr, "%d", &index)
	if index < 1 || index > len(ifaces) {
		Error(T("invalid_select"))
		return false
	}
	defaultIface := ifaces[index-1]
	Success(fmt.Sprintf(T("selected_nic"), defaultIface))
	Info(fmt.Sprintf(T("detect_ipv4"), defaultIface))
	currentMode, useStatic, currentIP, currentMask, currentGW := DetectInterfaceIPMode(defaultIface)
	var ipv4Addr, ipv4Netmask, ipv4Gateway string
	if currentIP != "" {
		if currentMode == "dhcp" {
			if ReadConfirm(T("switch_static_prompt"), false) {
				useStatic = true
				ipv4Addr, ipv4Netmask, ipv4Gateway = PromptIPv4Config(currentIP, currentMask, currentGW)
			} else {
				ipv4Addr = currentIP
				ipv4Netmask = currentMask
				ipv4Gateway = currentGW
				useStatic = false
				Info(T("keep_dhcp"))
			}
		} else {
			if ReadConfirm(fmt.Sprintf(T("reconfig_static_prompt"), currentIP, currentMask), false) {
				useStatic = true
				ipv4Addr, ipv4Netmask, ipv4Gateway = PromptIPv4Config(currentIP, currentMask, currentGW)
			} else {
				ipv4Addr = currentIP
				ipv4Netmask = currentMask
				ipv4Gateway = currentGW
				Info(fmt.Sprintf(T("keep_static"), currentIP, currentMask, currentGW))
			}
		}
	} else {
		Info(T("start_manual_setup"))
		useStatic = true
		ipv4Addr, ipv4Netmask, ipv4Gateway = PromptIPv4Config("", "", "")
	}
	if InputClosed() {
		Info(T("cancelled"))
		return false
	}
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
	fmt.Println()
	if !ReadConfirm(T("apply_confirm"), true) {
		Info(T("cancelled"))
		return false
	}

	// 记录旧 IPv6 地址，供在线切换时精确清理
	oldIPv6 := GetConfiguredIPv6Address(defaultIface)

	// ========== 关键修复：在配置新 IP 之前清理 bond0 ==========
	// 这是解决从 bond 切换到单网卡时 IP 冲突的核心修复
	if RunCmdSilent("ip", "link", "show", "bond0") == nil {
		Info(T("bond0_residual_clean"))

		// 1. 先添加 SSH 回程路由保护（避免清理 bond0 时断开 SSH）
		sshPeerIP := GetCurrentSSHPeerIP()
		sshDev := GetRouteDevForIP(sshPeerIP)
		oldGW := GetDefaultGateway()

		if sshPeerIP != "" && sshDev != "" {
			Info(T("ssh_route_protection"))
			_ = RunCmdSilent("ip", "route", "add", sshPeerIP+"/32", "via", oldGW, "dev", sshDev)
		}

		// 2. 检查 SSH 是否通过 bond0 连接
		if sshDev == "bond0" {
			Warn(T("bond0_ssh_skip"))
			Warn(T("bond0_manual_tip"))
			// 即使 SSH 通过 bond0，也要清理 bond0 的 IP，避免 IP 冲突
			// 但不删除 bond0 接口本身，保持 SSH 连接
			_ = RunCmdSilent("ip", "-4", "addr", "flush", "dev", "bond0")
			_ = RunCmdSilent("ip", "-6", "addr", "flush", "dev", "bond0")
			Info(T("bond0_ip_flushed"))
		} else {
			// SSH 不通过 bond0，可以安全删除 bond0
			_ = RunCmdSilent("ip", "-4", "addr", "flush", "dev", "bond0")
			_ = RunCmdSilent("ip", "-6", "addr", "flush", "dev", "bond0")
			CleanBondResidual()
			Success(T("bond0_cleaned"))
		}
	}

	backup := BackupFile(interfacesPath)
	Info(T("write_config"))
	if err := WriteSingleConfig(defaultIface, useStatic, ipv4Addr, ipv4Netmask, ipv4Gateway, configIPv6, ipv6Addr, ipv6Gateway); err != nil {
		Error(fmt.Sprintf(T("write_fail"), err))
		RestoreFile(backup, interfacesPath)
		return false
	}
	if !ValidateConfig(defaultIface) {
		// 校验未通过或用户放弃：回滚磁盘配置，避免留下起不来的 interfaces
		RestoreFile(backup, interfacesPath)
		return false
	}
	Success(T("config_written"))
	ConfigureDNS(defaultIface, configIPv6)
	if useStatic {
		err = ApplyIPv4Online(defaultIface, ipv4Addr, ipv4Netmask, ipv4Gateway, currentIP)
		if err != nil {
			Warn(T("hot_apply_fail_fallback"))
			_ = RunCmdSilent("ifdown", defaultIface)
			_ = RunCmdSilent("ifup", defaultIface)
		}
	} else {
		Info(T("apply_network"))
		_ = RunCmdSilent("ifdown", defaultIface)
		_ = RunCmdSilent("ifup", defaultIface)
	}
	if configIPv6 {
		_ = ApplyIPv6Online(defaultIface, ipv6Addr, ipv6Gateway, oldIPv6)
	}
	// 新配置已生效后才清理其它物理网卡的残留 IP。
	// 放在最后可确保前面任何失败路径都不会先把别的网卡清空导致失联。
	CleanOtherInterfaces(defaultIface)
	Sleep(2)
	if out, err := RunCmd("ip", "link", "show", defaultIface); err == nil && (strings.Contains(out, "state UP") || strings.Contains(out, "LOWER_UP")) {
		Success(fmt.Sprintf(T("nic_is_up"), defaultIface))
	} else {
		Error(fmt.Sprintf(T("nic_is_down"), defaultIface))
		if ReadConfirm(T("restart_networking"), true) {
			_ = RunCmdSilent("systemctl", "restart", "networking")
			Success(T("networking_restarted"))
		}
	}
	fmt.Println()
	Info(T("final_verify"))
	fmt.Printf(T("verify_interface")+"\n", defaultIface)
	modeStr := T("mode_dhcp")
	if useStatic {
		modeStr = T("mode_static")
	}
	fmt.Printf(T("verify_ipv4_mode")+"\n", modeStr)
	fmt.Printf(T("verify_active_ipv4")+"\n", GetInterfaceIPv4CIDR(defaultIface))
	fmt.Printf(T("verify_gw")+"\n", GetDefaultGateway())
	if configIPv6 {
		fmt.Printf(T("verify_active_ipv6")+"\n", GetInterfaceIPv6Global(defaultIface))
	}
	dnsOut, _ := os.ReadFile("/etc/resolv.conf")
	var dnsList []string
	for _, line := range strings.Split(string(dnsOut), "\n") {
		if strings.HasPrefix(line, "nameserver ") {
			dnsList = append(dnsList, strings.TrimPrefix(line, "nameserver "))
		}
	}
	fmt.Printf(T("verify_dns")+"\n", strings.Join(dnsList, " "))
	fmt.Println()
	Success(T("single_complete"))
	return true
}

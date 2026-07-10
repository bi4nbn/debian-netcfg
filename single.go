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
	Info(T("detect_nics"))
	ifaces := ListPhysicalInterfaces()
	if len(ifaces) == 0 {
		Fatal(T("no_valid_nics"))
	}
	Info(T("available_nics"))
	for i, iface := range ifaces {
		fmt.Printf("  %d. %s\n", i+1, iface)
	}
	indexStr := ReadInput(T("select_nic_prompt"), "1")
	// 输入0返回主菜单
	if indexStr == "0" {
		Info(T("cancelled"))
		return false
	}
	index := 1
	fmt.Sscanf(indexStr, "%d", &index)
	if index < 1 || index > len(ifaces) {
		Fatal(T("invalid_select"))
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
	fmt.Println()
	if !ReadConfirm(T("apply_confirm"), true) {
		Info(T("cancelled"))
		return false
	}
	BackupFile(interfacesPath)
	Info(T("write_config"))
	err := WriteSingleConfig(defaultIface, useStatic, ipv4Addr, ipv4Netmask, ipv4Gateway, configIPv6, ipv6Addr, ipv6Gateway)
	if err != nil {
		Fatal(fmt.Sprintf(T("write_fail"), err))
	}
	if !ValidateConfig(defaultIface) {
		return false
	}
	_ = os.Chmod(interfacesPath, 0644)
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
		_ = ApplyIPv6Online(defaultIface, ipv6Addr, ipv6Gateway)
	}
	Sleep(2)
	if out, err := RunCmd("ip", "link", "show", defaultIface); err == nil && strings.Contains(out, "state UP") {
		Success(fmt.Sprintf(T("nic_is_up"), defaultIface))
	} else {
		Error(fmt.Sprintf(T("nic_is_down"), defaultIface))
		if ReadConfirm(T("restart_networking"), true) {
			_ = RunCmdSilent("systemctl", "restart", "networking")
			Success(T("networking_restarted"))
		}
	}
	if RunCmdSilent("ip", "link", "show", "bond0") == nil {
		sshPeerIP := GetCurrentSSHPeerIP()
		sshDev := GetRouteDevForIP(sshPeerIP)
		if sshPeerIP == "" || sshDev != "bond0" {
			Info(T("bond0_residual_clean"))
			_ = RunCmdSilent("ip", "-4", "addr", "flush", "dev", "bond0")
			_ = RunCmdSilent("ip", "-6", "addr", "flush", "dev", "bond0")
			CleanBondResidual()
			Success(T("bond0_cleaned"))
		} else {
			Warn(T("bond0_ssh_skip"))
			Warn(T("bond0_manual_tip"))
		}
	}
	CleanOtherInterfaces(defaultIface)
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

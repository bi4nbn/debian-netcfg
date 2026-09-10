package main

import (
	"fmt"
)

func IPv6OnlyConfig() bool {
	fmt.Println(T("ipv6_only_title"))
	fmt.Println()
	CheckRoot()
	if err := EnsureDependencies(); err != nil {
		Error(err.Error())
		return false
	}
	Info(T("detect_nics"))
	ifaces, err := ListAllInterfaces()
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
	targetIface := ifaces[index-1]
	Success(fmt.Sprintf(T("selected_nic"), targetIface))
	ipv6Addr, ipv6Gateway := PromptIPv6Config()
	if InputClosed() {
		Info(T("cancelled"))
		return false
	}
	fmt.Println()
	if !ReadConfirm(T("apply_confirm"), true) {
		Info(T("cancelled"))
		return false
	}

	// 记录旧地址，在线切换时只删除本工具管理的这一个地址
	oldIPv6 := GetConfiguredIPv6Address(targetIface)

	backup := BackupFile(interfacesPath)
	Info(T("write_config"))
	if err := AddIPv6ToConfig(targetIface, ipv6Addr, ipv6Gateway); err != nil {
		Error(fmt.Sprintf(T("write_fail"), err))
		RestoreFile(backup, interfacesPath)
		return false
	}
	if !ValidateConfig(targetIface) {
		// 校验未通过或用户放弃：回滚磁盘配置
		RestoreFile(backup, interfacesPath)
		return false
	}
	Success(T("config_written"))
	if err := ApplyIPv6Online(targetIface, ipv6Addr, ipv6Gateway, oldIPv6); err != nil {
		Warn(T("ipv6_gw_warn"))
	}
	ConfigureDNS(targetIface, true)
	Sleep(2)
	fmt.Println()
	Info(T("final_verify"))
	fmt.Printf(T("verify_interface")+"\n", targetIface)
	fmt.Printf(T("verify_active_ipv6")+"\n", GetInterfaceIPv6Global(targetIface))
	fmt.Println()
	Success(T("ipv6_standalone_complete"))
	return true
}

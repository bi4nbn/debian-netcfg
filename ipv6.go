package main

import (
	"fmt"
	"os"
)

func IPv6OnlyConfig() bool {
	fmt.Println(T("ipv6_only_title"))
	fmt.Println()
	CheckRoot()
	Info(T("detect_nics"))
	ifaces := ListAllInterfaces()
	if len(ifaces) == 0 {
		Fatal(T("no_valid_nics"))
	}
	Info(T("available_nics"))
	for i, iface := range ifaces {
		fmt.Printf("  %d. %s\n", i+1, iface)
	}
	indexStr := ReadInput(T("select_nic_prompt"), "1")
	if indexStr == "0" {
		Info(T("cancelled"))
		return false
	}
	index := 1
	fmt.Sscanf(indexStr, "%d", &index)
	if index < 1 || index > len(ifaces) {
		Fatal(T("invalid_select"))
	}
	targetIface := ifaces[index-1]
	Success(fmt.Sprintf(T("selected_nic"), targetIface))
	ipv6Addr, ipv6Gateway := PromptIPv6Config()
	fmt.Println()
	if !ReadConfirm(T("apply_confirm"), true) {
		Info(T("cancelled"))
		return false
	}
	BackupFile(interfacesPath)
	Info(T("write_config"))
	err := AddIPv6ToConfig(targetIface, ipv6Addr, ipv6Gateway)
	if err != nil {
		Fatal(fmt.Sprintf(T("write_fail"), err))
	}
	if !ValidateConfig(targetIface) {
		return false
	}
	_ = os.Chmod(interfacesPath, 0644)
	Success(T("config_written"))
	err = ApplyIPv6Online(targetIface, ipv6Addr, ipv6Gateway)
	if err != nil {
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

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

func ListPhysicalInterfaces() []string {
	out, err := RunCmd("ip", "-br", "link", "show")
	if err != nil {
		Fatal(T("list_nic_fail"))
	}
	var ifaces []string
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		if strings.Contains(name, "lo") ||
			strings.Contains(name, "docker") ||
			strings.Contains(name, "veth") ||
			strings.Contains(name, "tap") ||
			strings.Contains(name, "tun") ||
			strings.Contains(name, "bond") ||
			strings.Contains(name, "br-") ||
			strings.Contains(name, "ifb") {
			continue
		}
		ifaces = append(ifaces, name)
	}
	return ifaces
}

func ListAllInterfaces() []string {
	out, err := RunCmd("ip", "-br", "link", "show")
	if err != nil {
		Fatal(T("list_nic_fail"))
	}
	var ifaces []string
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := strings.TrimSuffix(fields[0], ":")
		if strings.Contains(name, "lo") ||
			strings.Contains(name, "docker") ||
			strings.Contains(name, "veth") ||
			strings.Contains(name, "tap") ||
			strings.Contains(name, "tun") ||
			strings.Contains(name, "br-") {
			continue
		}
		ifaces = append(ifaces, name)
	}
	return ifaces
}

func GetInterfaceStatus(iface string) string {
	out, err := RunCmd("ip", "link", "show", iface)
	if err != nil {
		return "UNKNOWN"
	}
	if strings.Contains(out, "state UP") {
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

func DetectInterfaceIPMode(iface string) (mode string, useStatic bool, currentIP, currentMask, currentGW string) {
	mode = ""
	useStatic = true

	if _, err := os.Stat(interfacesPath); err == nil {
		data, _ := os.ReadFile(interfacesPath)
		content := string(data)
		if strings.Contains(content, fmt.Sprintf("iface %s inet dhcp", iface)) {
			mode = "dhcp"
			useStatic = false
			Info(T("current_dhcp"))
		} else if strings.Contains(content, fmt.Sprintf("iface %s inet static", iface)) {
			mode = "static"
			useStatic = true
			Info(T("current_static"))
		}
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
	prefixLen, _ := mask.Size()
	newCIDR := fmt.Sprintf("%s/%d", newIP, prefixLen)

	sshPeerIP := GetCurrentSSHPeerIP()
	oldGW := GetDefaultGateway()
	sshDev := GetRouteDevForIP(sshPeerIP)

	if sshPeerIP != "" && sshDev != "" && sshDev != iface {
		Info(T("ssh_route_protection"))
		_ = RunCmdSilent("ip", "route", "add", sshPeerIP+"/32", "via", oldGW, "dev", sshDev)
	}

	err := RunCmdSilent("ip", "addr", "add", newCIDR, "dev", iface)
	if err != nil {
		return fmt.Errorf("failed to add new IP: %v", err)
	}
	Success(fmt.Sprintf(T("new_ip_bound"), newIP, iface))

	if newGW != "" && newGW != oldGW {
		err = RunCmdSilent("ip", "route", "add", "default", "via", newGW, "dev", iface, "metric", "100")
		if err != nil {
			Warn(T("new_gw_fail"))
		} else {
			if oldGW != "" {
				_ = RunCmdSilent("ip", "route", "del", "default", "via", oldGW)
			}
			_ = RunCmdSilent("ip", "route", "change", "default", "via", newGW, "dev", iface, "metric", "0")
			Success(fmt.Sprintf(T("gw_applied"), newGW))
		}
	}

	if oldIP != "" && oldIP != newIP {
		Sleep(1)
		_ = RunCmdSilent("ip", "addr", "del", oldIP, "dev", iface)
		Info(fmt.Sprintf(T("old_ip_removed"), oldIP))
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

	allNics := ListPhysicalInterfaces()
	for _, nic := range allNics {
		if nic == keepIface {
			continue
		}
		if nic == sshDev {
			skipped = nic
			continue
		}
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

func ConfigureDNS(targetIface string, enableIPv6 bool) {
	Info(T("config_dns"))
	resolvBackup := fmt.Sprintf("/etc/resolv.conf.bak_%s", time.Now().Format("20060102_150405"))
	if _, err := os.Stat("/etc/resolv.conf"); err == nil {
		data, _ := os.ReadFile("/etc/resolv.conf")
		_ = os.WriteFile(resolvBackup, data, 0644)
	}
	Success(fmt.Sprintf(T("backup_dns"), resolvBackup))
	_ = RunCmdSilent("chattr", "-i", "/etc/resolv.conf")
	if CommandExists("resolvectl") {
		args := append([]string{"dns", targetIface}, AliDNS4...)
		_ = RunCmdSilent("resolvectl", args...)
		if enableIPv6 {
			args6 := append([]string{"dns", targetIface}, AliDNS6...)
			_ = RunCmdSilent("resolvectl", args6...)
		}
		Success(T("dns_via_resolvectl"))
	} else {
		var content string
		for _, dns := range AliDNS4 {
			content += fmt.Sprintf("nameserver %s\n", dns)
		}
		if enableIPv6 {
			for _, dns := range AliDNS6 {
				content += fmt.Sprintf("nameserver %s\n", dns)
			}
		}
		_ = os.WriteFile("/etc/resolv.conf", []byte(content), 0644)
		Success(T("dns_written"))
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

func ApplyIPv6Online(iface, ipv6Addr, ipv6GW string) error {
	Info(T("apply_ipv6_online"))

	if GetInterfaceStatus(iface) != "UP" {
		_ = RunCmdSilent("ip", "link", "set", iface, "up")
		Sleep(1)
	}

	_ = RunCmdSilent("ip", "-6", "addr", "flush", "dev", iface, "scope", "global")
	err := RunCmdSilent("ip", "-6", "addr", "add", ipv6Addr, "dev", iface)
	if err != nil {
		Error(T("ipv6_addr_fail"))
		return err
	}
	Success(fmt.Sprintf(T("ipv6_addr_added"), ipv6Addr, iface))
	_ = RunCmdSilent("ip", "-6", "route", "del", "default", "dev", iface)
	err = RunCmdSilent("ip", "-6", "route", "add", "default", "via", ipv6GW, "dev", iface)
	if err != nil {
		Warn(T("ipv6_gw_fail"))
		Warn(T("ipv6_gw_fail_tip"))
	} else {
		Success(fmt.Sprintf(T("ipv6_gw_added"), ipv6GW))
	}
	return nil
}

func WriteSingleConfig(iface string, useStatic bool, ip, netmask, gateway string, enableIPv6 bool, ipv6Addr, ipv6GW string) error {
	content := fmt.Sprintf("# Auto generated config - %s\n", time.Now().Format("2006-01-02_15:04:05"))
	content += "auto lo\niface lo inet loopback\n\n"
	content += fmt.Sprintf("auto %s\n", iface)
	if !useStatic {
		content += fmt.Sprintf("iface %s inet dhcp\n", iface)
	} else {
		content += fmt.Sprintf("iface %s inet static\n", iface)
		content += fmt.Sprintf("    address %s\n", ip)
		content += fmt.Sprintf("    netmask %s\n", netmask)
		content += fmt.Sprintf("    gateway %s\n", gateway)
	}
	if enableIPv6 {
		content += "\n"
		content += fmt.Sprintf("iface %s inet6 static\n", iface)
		content += fmt.Sprintf("    address %s\n", ipv6Addr)
		content += fmt.Sprintf("    gateway %s\n", ipv6GW)
	}
	return os.WriteFile(interfacesPath, []byte(content), 0644)
}

func WriteBondConfig(nics []string, ip, netmask, gateway string, mode string, enableIPv6 bool, ipv6Addr, ipv6GW string) error {
	content := fmt.Sprintf("# Auto generated bond config - %s\n", time.Now().Format("2006-01-02_15:04:05"))
	content += "auto lo\niface lo inet loopback\n\n"
	for _, nic := range nics {
		content += fmt.Sprintf("auto %s\n", nic)
		content += fmt.Sprintf("iface %s inet manual\n", nic)
		content += "    bond-master bond0\n\n"
	}
	content += "auto bond0\n"
	content += "iface bond0 inet static\n"
	content += fmt.Sprintf("    address %s\n", ip)
	content += fmt.Sprintf("    netmask %s\n", netmask)
	content += fmt.Sprintf("    gateway %s\n", gateway)
	content += fmt.Sprintf("    dns-nameservers %s\n", strings.Join(AliDNS4, " "))
	content += fmt.Sprintf("    bond-mode %s\n", mode)
	content += "    bond-miimon 100\n"
	content += fmt.Sprintf("    bond-slaves %s\n", strings.Join(nics, " "))
	if mode == "802.3ad" {
		content += "    bond-lacp-rate fast\n"
		content += "    bond-xmit-hash-policy layer3+4\n"
	}
	if enableIPv6 {
		content += "\n"
		content += "iface bond0 inet6 static\n"
		content += fmt.Sprintf("    address %s\n", ipv6Addr)
		content += fmt.Sprintf("    gateway %s\n", ipv6GW)
	}
	return os.WriteFile(interfacesPath, []byte(content), 0644)
}

func AddIPv6ToConfig(iface, ipv6Addr, ipv6GW string) error {
	data, err := os.ReadFile(interfacesPath)
	if err != nil {
		return err
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	var newLines []string
	skip := false
	for _, line := range lines {
		if strings.HasPrefix(line, fmt.Sprintf("iface %s inet6 static", iface)) {
			skip = true
			continue
		}
		if skip && (strings.HasPrefix(line, "    ") || line == "") {
			continue
		}
		skip = false
		newLines = append(newLines, line)
	}
	var insertIdx = -1
	inBlock := false
	for i, line := range newLines {
		if strings.HasPrefix(line, fmt.Sprintf("iface %s inet ", iface)) {
			inBlock = true
			continue
		}
		if inBlock {
			if line == "" || !strings.HasPrefix(line, "    ") {
				insertIdx = i
				break
			}
		}
	}
	if insertIdx == -1 {
		newLines = append(newLines, "")
		newLines = append(newLines, fmt.Sprintf("iface %s inet6 static", iface))
		newLines = append(newLines, fmt.Sprintf("    address %s", ipv6Addr))
		newLines = append(newLines, fmt.Sprintf("    gateway %s", ipv6GW))
	} else {
		ipv6Block := []string{
			"",
			fmt.Sprintf("iface %s inet6 static", iface),
			fmt.Sprintf("    address %s", ipv6Addr),
			fmt.Sprintf("    gateway %s", ipv6GW),
		}
		newLines = append(newLines[:insertIdx], append(ipv6Block, newLines[insertIdx:]...)...)
	}
	finalContent := strings.Join(newLines, "\n")
	err = os.WriteFile(interfacesPath, []byte(finalContent), 0644)
	if err == nil {
		Success(fmt.Sprintf(T("ipv6_config_updated"), iface))
	}
	return err
}

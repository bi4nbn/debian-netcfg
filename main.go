package main

import (
	"fmt"
)

func showMenu() {
	fmt.Print("\033[H\033[2J") // 清屏
	fmt.Println("======================================")
	fmt.Printf("  %s\n", T("menu_title"))
	fmt.Println("======================================")
	fmt.Println("  " + T("menu_opt1"))
	fmt.Println("  " + T("menu_opt2"))
	fmt.Println("  " + T("menu_opt3"))
	fmt.Println("  " + T("menu_opt4"))
	fmt.Println("  " + T("menu_opt5"))
	fmt.Println("  " + T("menu_opt0"))
	fmt.Println("======================================")
	fmt.Println()
	// 当前网络概览
	fmt.Printf("%s%s%s\n", BLUE, T("net_overview"), NC)
	physNics := ListPhysicalInterfaces()
	if len(physNics) > 0 {
		fmt.Println(T("phys_nics"))
		for _, nic := range physNics {
			status := GetInterfaceStatus(nic)
			statusColor := RED
			if status == "UP" {
				statusColor = GREEN
			}
			ip4 := GetInterfaceIPv4CIDR(nic)
			if ip4 == "" {
				ip4 = T("no_ip")
			}
			ip6 := GetInterfaceIPv6Global(nic)
			fmt.Printf("  %s%-4s%s %-10s IPv4: %-18s IPv6: %s\n", statusColor, status, NC, nic, ip4, ip6)
		}
	}
	// Bond0 状态
	if RunCmdSilent("ip", "link", "show", "bond0") == nil {
		status := GetInterfaceStatus("bond0")
		statusColor := RED
		if status == "UP" {
			statusColor = GREEN
		}
		bondIP4 := GetInterfaceIPv4CIDR("bond0")
		if bondIP4 == "" {
			bondIP4 = T("no_ip")
		}
		bondIP6 := GetInterfaceIPv6Global("bond0")
		fmt.Printf("  %s%-4s%s %-10s IPv4: %-18s IPv6: %s\n", statusColor, status, NC, "bond0", bondIP4, bondIP6)
	}
	fmt.Printf("%s %s\n", T("default_gw4"), GetDefaultGateway())
	fmt.Printf("%s %s\n", T("default_gw6"), GetDefaultIPv6Gateway())
	fmt.Println("======================================")
	fmt.Println()
}

func switchLanguage() {
	fmt.Print("\033[H\033[2J")
	fmt.Println(T("switch_lang_title"))
	fmt.Println()
	fmt.Println(T("select_lang"))
	fmt.Println("  " + T("lang_en"))
	fmt.Println("  " + T("lang_zh"))
	fmt.Println()
	choice := ReadInput(T("lang_prompt"), "1")
	switch choice {
	case "2":
		currentLang = "zh"
	default:
		currentLang = "en"
	}
	Success(T("lang_switched"))
	Sleep(1)
}

func main() {
	for {
		showMenu()
		choice := ReadInput(T("menu_prompt"), "")
		switch choice {
		case "1":
			fmt.Print("\033[H\033[2J")
			if SingleNicConfig() {
				fmt.Println()
				ReadInput(T("press_enter_menu"), "")
			}
		case "2":
			fmt.Print("\033[H\033[2J")
			if BondConfig() {
				fmt.Println()
				ReadInput(T("press_enter_menu"), "")
			}
		case "3":
			fmt.Print("\033[H\033[2J")
			if IPv6OnlyConfig() {
				fmt.Println()
				ReadInput(T("press_enter_menu"), "")
			}
		case "4":
			fmt.Print("\033[H\033[2J")
			NetworkDNSTest()
		case "5":
			switchLanguage()
		case "0":
			Info(T("menu_exit"))
			return
		default:
			Error(T("menu_invalid"))
			Sleep(1)
		}
	}
}

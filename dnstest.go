package main

import (
	"fmt"
	"strings"
)

func NetworkDNSTest() {
	fmt.Println(T("dns_test_title"))
	fmt.Println()

	Info(T("test_ipv4_dns"))
	fmt.Println()
	for _, dns := range AliDNS4 {
		fmt.Printf("  %s ... ", dns)
		out, err := RunCmd("ping", "-4", "-c", "2", "-W", "2", dns)
		if err == nil {
			avg := ""
			lines := strings.Split(out, "\n")
			for _, line := range lines {
				if strings.Contains(line, "rtt") || strings.Contains(line, "min/avg/max") {
					parts := strings.Split(line, "/")
					if len(parts) >= 5 {
						avg = parts[4]
						break
					}
				}
			}
			if avg != "" {
				fmt.Printf("%s%s%s %s\n", GREEN, T("dns_test_ok"), NC, fmt.Sprintf(T("dns_test_avg"), avg))
			} else {
				fmt.Printf("%s%s%s %s\n", GREEN, T("dns_test_ok"), NC, T("dns_test_connected"))
			}
		} else {
			fmt.Printf("%s%s%s\n", RED, T("dns_test_fail"), NC)
		}
	}

	fmt.Println()
	Info(T("test_ipv6_dns"))
	fmt.Println()
	for _, dns := range AliDNS6 {
		fmt.Printf("  %s ... ", dns)
		out, err := RunCmd("ping", "-6", "-c", "2", "-W", "2", dns)
		if err == nil {
			avg := ""
			lines := strings.Split(out, "\n")
			for _, line := range lines {
				if strings.Contains(line, "rtt") || strings.Contains(line, "min/avg/max") {
					parts := strings.Split(line, "/")
					if len(parts) >= 5 {
						avg = parts[4]
						break
					}
				}
			}
			if avg != "" {
				fmt.Printf("%s%s%s %s\n", GREEN, T("dns_test_ok"), NC, fmt.Sprintf(T("dns_test_avg"), avg))
			} else {
				fmt.Printf("%s%s%s %s\n", GREEN, T("dns_test_ok"), NC, T("dns_test_connected"))
			}
		} else {
			fmt.Printf("%s%s%s\n", YELLOW, T("dns_test_ipv6_fail"), NC)
		}
	}

	fmt.Println()
	Success(T("dns_test_complete"))
	fmt.Println()
	ReadInput(T("press_enter_menu"), "")
}

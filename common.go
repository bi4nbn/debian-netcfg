package main
import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)
// 颜色定义
const (
	RED    = "\033[0;31m"
	GREEN  = "\033[0;32m"
	YELLOW = "\033[1;33m"
	BLUE   = "\033[0;34m"
	NC     = "\033[0m"
)
// 全局 DNS 配置（阿里云）
var (
	AliDNS4 = []string{"223.5.5.5", "223.6.6.6"}
	AliDNS6 = []string{"2400:3200::1", "2400:3200:baba::1"}
)
// ------------------------------
// 日志函数
// ------------------------------
func Error(msg string)    { fmt.Fprintf(os.Stderr, "%s[Error]%s %s\n", RED, NC, msg) }
func Fatal(msg string)    { fmt.Fprintf(os.Stderr, "%s[Fatal]%s %s\n", RED, NC, msg); os.Exit(1) }
func Info(msg string)     { fmt.Printf("%s[Info]%s %s\n", BLUE, NC, msg) }
func Success(msg string)  { fmt.Printf("%s[Success]%s %s\n", GREEN, NC, msg) }
func Warn(msg string)     { fmt.Printf("%s[Warning]%s %s\n", YELLOW, NC, msg) }
func Interact(msg string) { fmt.Printf("%s[Prompt]%s %s", YELLOW, NC, msg) }
// ------------------------------
// 交互输入工具
// ------------------------------
var reader = bufio.NewReader(os.Stdin)
// ReadInput 读取用户输入，空则返回默认值
func ReadInput(prompt string, defaultValue string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}
// ReadConfirm 读取 y/n 确认，默认值可选
func ReadConfirm(prompt string, defaultYes bool) bool {
	defStr := "y"
	if !defaultYes {
		defStr = "n"
	}
	input := ReadInput(prompt, defStr)
	return strings.ToLower(input) == "y"
}
// PromptIPv4Config 统一IPv4 CIDR输入，自动换算掩码、自动识别网关，支持手动修改
func PromptIPv4Config(defaultIP, defaultMask, defaultGW string) (ip, mask, gw string) {
	// 回填旧配置为CIDR格式
	defaultCIDR := ""
	if defaultIP != "" && defaultMask != "" {
		cidrNum, err := NetmaskToCIDR(defaultMask)
		if err == nil {
			defaultCIDR = fmt.Sprintf("%s/%d", defaultIP, cidrNum)
		}
	}

	// 1. 仅输入CIDR，循环校验格式
	var inputCIDR string
	for {
		inputCIDR = ReadInput(T("input_ipv4"), defaultCIDR)
		if inputCIDR == "" {
			inputCIDR = defaultCIDR
		}
		_, _, err := net.ParseCIDR(inputCIDR)
		if err != nil {
			Error(T("invalid_ipv4"))
			continue
		}
		break
	}

	// 拆分IP、自动换算掩码
	parts := strings.Split(inputCIDR, "/")
	ip = parts[0]
	var prefix int
	fmt.Sscanf(parts[1], "%d", &prefix)
	mask = CIDRToNetmask(prefix)

	// 2. 自动获取网关
	autoGW, err := GetAutoGatewayFromCIDR(inputCIDR)
	if err == nil && ValidateIPv4(autoGW) {
		gw = autoGW
	} else {
		// 推算失败，使用系统现有默认网关兜底
		gw = GetDefaultGateway()
	}
	// 兜底防止网关为空字符串写入配置
	if gw == "" {
		gw = "0.0.0.0"
	}

	// 3. 询问是否手动修改网关
	if ReadConfirm(fmt.Sprintf(T("auto_gw_confirm"), gw), false) {
		// 用户选择手动修改，进入网关输入循环
		for {
			inputGW := ReadInput(T("input_gw"), gw)
			if inputGW == "" {
				inputGW = gw
			}
			if ValidateIPv4(inputGW) {
				gw = inputGW
				break
			}
			Error(T("invalid_gw"))
		}
	}

	Info(fmt.Sprintf(T("ipv4_set"), ip, mask, gw))
	return
}

// PromptIPv6Config 统一IPv6地址/网关交互输入，全局复用同一套校验逻辑
func PromptIPv6Config() (addr, gw string) {
	for {
		addr = ReadInput(T("input_ipv6_addr"), "")
		if ValidateIPv6CIDR(addr) {
			break
		}
		Error(T("invalid_ipv6"))
	}
	for {
		gw = ReadInput(T("input_ipv6_gw"), "")
		if ValidateIPv6(gw) {
			break
		}
		Error(T("invalid_ipv6"))
	}
	Info(fmt.Sprintf(T("ipv6_set"), addr, gw))
	return
}
// ------------------------------
// 系统工具
// ------------------------------
// CheckRoot 检查是否 root 运行
func CheckRoot() {
	if os.Getuid() != 0 {
		Fatal(T("err_run_root"))
	}
}
// CommandExists 检查命令是否存在
func CommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
// RunCmd 执行命令，返回输出和错误
func RunCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
// RunCmdSilent 静默执行命令，失败返回错误
func RunCmdSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
// CheckAptNetwork 检查 apt 源网络连通性
func CheckAptNetwork() {
	err1 := RunCmdSilent("ping", "-c", "1", "-W", "2", "deb.debian.org")
	err2 := RunCmdSilent("ping", "-c", "1", "-W", "2", "mirrors.aliyun.com")
	if err1 != nil && err2 != nil {
		Fatal(T("no_network_apt"))
	}
}
// InstallBaseDeps 安装基础依赖
func InstallBaseDeps() {
	var needInstall []string
	if !CommandExists("ip") {
		needInstall = append(needInstall, "iproute2")
	}
	if !CommandExists("ifup") || !CommandExists("ifdown") {
		needInstall = append(needInstall, "ifupdown")
	}
	if len(needInstall) > 0 {
		err := RunCmdSilent("apt", "update", "-qq")
		if err != nil {
			Fatal(T("apt_update_fail"))
		}
		args := append([]string{"install", "-y", "-qq"}, needInstall...)
		err = RunCmdSilent("apt", args...)
		if err != nil {
			Fatal(T("apt_install_fail"))
		}
	}
	if !CommandExists("ip") {
		Fatal(T("ip_unavailable"))
	}
	if !CommandExists("ifup") || !CommandExists("ifdown") {
		Fatal(T("ifupdown_unavailable"))
	}
}
// DisableConflictServices 禁用冲突的网络服务
func DisableConflictServices() {
	if RunCmdSilent("systemctl", "is-active", "--quiet", "NetworkManager") == nil {
		_ = RunCmdSilent("systemctl", "stop", "NetworkManager")
		_ = RunCmdSilent("systemctl", "disable", "NetworkManager")
	}
	if RunCmdSilent("systemctl", "is-active", "--quiet", "systemd-networkd") == nil {
		_ = RunCmdSilent("systemctl", "stop", "systemd-networkd")
		_ = RunCmdSilent("systemctl", "disable", "systemd-networkd")
	}
}
// Sleep 延时等待
func Sleep(seconds int) {
	time.Sleep(time.Duration(seconds) * time.Second)
}
// GetCurrentSSHLocalIP 获取当前SSH会话的本机服务端IP
func GetCurrentSSHLocalIP() string {
	conn := os.Getenv("SSH_CONNECTION")
	if conn == "" {
		return ""
	}
	parts := strings.Fields(conn)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}
// GetCurrentSSHPeerIP 获取当前SSH会话的客户端IP，用于路由保护
func GetCurrentSSHPeerIP() string {
	conn := os.Getenv("SSH_CONNECTION")
	if conn == "" {
		return ""
	}
	parts := strings.Fields(conn)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}
// GetRouteDevForIP 查询指定IP走哪个网卡出口
func GetRouteDevForIP(ip string) string {
	if ip == "" {
		return ""
	}
	out, err := RunCmd("ip", "route", "get", ip)
	if err != nil {
		return ""
	}
	fields := strings.Fields(out)
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}
// ------------------------------
// IP 工具函数
// ------------------------------
// CIDRToNetmask CIDR 转点分十进制子网掩码
func CIDRToNetmask(cidr int) string {
	if cidr < 0 || cidr > 32 {
		return "255.255.0.0"
	}
	mask := net.CIDRMask(cidr, 32)
	return net.IP(mask).String()
}

// NetmaskToCIDR 点分十进制掩码转CIDR前缀长度
func NetmaskToCIDR(maskStr string) (int, error) {
	ip := net.ParseIP(maskStr).To4()
	if ip == nil {
		return 0, fmt.Errorf("invalid netmask")
	}
	mask := net.IPMask(ip)
	ones, bits := mask.Size()
	if bits != 32 {
		return 0, fmt.Errorf("not ipv4 mask")
	}
	return ones, nil
}

// GetAutoGatewayFromCIDR 根据CIDR算出网段网络地址，网络地址+1作为自动网关
func GetAutoGatewayFromCIDR(cidrStr string) (string, error) {
	// 标准解析：ipNet.IP 是完整网段网络地址
	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return "", err
	}
	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return "", fmt.Errorf("only support ipv4 cidr")
	}
	// 复制网络地址，避免修改原切片
	gatewayIP := make(net.IP, 4)
	copy(gatewayIP, ip4)
	// 网络地址 +1 得到网段第一个可用主机地址（标准网关位置）
	gatewayIP[3] += 1
	return gatewayIP.String(), nil
}

// ValidateIPv4 校验 IPv4 地址格式
func ValidateIPv4(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() != nil
}
// ValidateNetmask 校验子网掩码合法性
func ValidateNetmask(mask string) bool {
	if !ValidateIPv4(mask) {
		return false
	}
	ipMask := net.IPMask(net.ParseIP(mask).To4())
	ones, bits := ipMask.Size()
	return bits == 32 && ones >= 0 && ones <= 32
}
// ParseNetmask 解析子网掩码，兼容点分格式(255.255.255.0)和CIDR前缀格式(24)
func ParseNetmask(input string) (string, error) {
	// 包含点则按点分十进制掩码处理
	if strings.Contains(input, ".") {
		if !ValidateIPv4(input) {
			return "", fmt.Errorf("invalid netmask format")
		}
		mask := net.ParseIP(input).To4()
		if mask == nil {
			return "", fmt.Errorf("invalid netmask")
		}
		// 校验掩码连续性：必须是连续的1后跟连续的0
		maskInt := uint32(mask[0])<<24 | uint32(mask[1])<<16 | uint32(mask[2])<<8 | uint32(mask[3])
		if maskInt != 0 {
			inv := ^maskInt + 1
			if inv&(inv-1) != 0 {
				return "", fmt.Errorf("non-contiguous netmask")
			}
		}
		return input, nil
	}
	// 纯数字按CIDR前缀长度处理
	var prefix int
	_, err := fmt.Sscanf(input, "%d", &prefix)
	if err != nil {
		return "", fmt.Errorf("invalid format")
	}
	if prefix < 0 || prefix > 32 {
		return "", fmt.Errorf("CIDR prefix must be 0-32")
	}
	mask := net.CIDRMask(prefix, 32)
	return net.IP(mask).String(), nil
}
// ValidateIPv6CIDR 严格校验 IPv6 CIDR 格式
func ValidateIPv6CIDR(addr string) bool {
	ip, _, err := net.ParseCIDR(addr)
	if err != nil {
		return false
	}
	return ip.To4() == nil // 确保是 IPv6
}
// ValidateIPv6 校验 IPv6 地址格式
func ValidateIPv6(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() == nil
}
// GetDefaultGateway 获取系统默认 IPv4 网关
func GetDefaultGateway() string {
	out, err := RunCmd("ip", "route", "show", "default")
	if err != nil {
		return ""
	}
	fields := strings.Fields(out)
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}
// IsDHCPClient 检查指定IP是否来自DHCP租约
func IsDHCPClient(ip string) bool {
	if ip == "" {
		return false
	}
	leaseFile := "/var/lib/dhcp/dhclient.leases"
	if _, err := os.Stat(leaseFile); os.IsNotExist(err) {
		return false
	}
	data, err := os.ReadFile(leaseFile)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), ip)
}
// ------------------------------
// 文件工具
// ------------------------------
// BackupFile 备份文件，添加时间戳后缀
func BackupFile(path string) string {
	backupPath := fmt.Sprintf("%s.bak_%s", path, time.Now().Format("20060102_150405"))
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err == nil {
			_ = os.WriteFile(backupPath, data, 0644)
			Success(fmt.Sprintf(T("backup_config"), backupPath))
		}
	}
	return backupPath
}
package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ================== 初始化系统（Go 实现） ==================

// getDebianCodename 获取 Debian 版本代号
func getDebianCodename() (string, error) {
	// 先尝试 /etc/os-release
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "VERSION_CODENAME=") {
				val := strings.TrimPrefix(line, "VERSION_CODENAME=")
				val = strings.Trim(val, `"`)
				if val != "" {
					return val, nil
				}
			}
		}
	}
	// 尝试 lsb_release 命令
	if out, err := RunCmd("lsb_release", "-cs"); err == nil {
		ver := strings.TrimSpace(out)
		if ver != "" {
			return ver, nil
		}
	}
	// 回退到 /etc/debian_version
	if data, err := os.ReadFile("/etc/debian_version"); err == nil {
		ver := strings.TrimSpace(string(data))
		parts := strings.Split(ver, ".")
		major := parts[0]
		switch major {
		case "11":
			return "bullseye", nil
		case "12":
			return "bookworm", nil
		case "13":
			return "trixie", nil
		default:
			return "", fmt.Errorf("unsupported Debian version: %s", major)
		}
	}
	return "", fmt.Errorf("cannot determine Debian codename")
}

// writeSourcesList 写入华为云源
func writeSourcesList(codename string) error {
	content := fmt.Sprintf(`deb https://mirrors.huaweicloud.com/debian/ %s main contrib non-free non-free-firmware
deb https://mirrors.huaweicloud.com/debian/ %s-updates main contrib non-free non-free-firmware
deb https://mirrors.huaweicloud.com/debian/ %s-backports main contrib non-free non-free-firmware
deb https://mirrors.huaweicloud.com/debian-security/ %s-security main contrib non-free non-free-firmware
`, codename, codename, codename, codename)
	if err := os.WriteFile("/etc/apt/sources.list", []byte(content), 0644); err != nil {
		return err
	}
	// 处理 DEB822 格式源（如果存在）
	if _, err := os.Stat("/etc/apt/sources.list.d/debian.sources"); err == nil {
		data, err := os.ReadFile("/etc/apt/sources.list.d/debian.sources")
		if err == nil {
			newData := strings.ReplaceAll(string(data), "deb.debian.org", "mirrors.huaweicloud.com")
			_ = os.WriteFile("/etc/apt/sources.list.d/debian.sources", []byte(newData), 0644)
		}
	}
	return nil
}

// runAptUpdateInstall 执行 apt update 并安装指定包
func runAptUpdateInstall(packages ...string) error {
	Info("Running apt update...")
	if out, err := RunCmd("apt", "update", "-y"); err != nil {
		return fmt.Errorf("apt update failed: %v, output: %s", err, out)
	}
	if len(packages) > 0 {
		Info("Installing packages: " + strings.Join(packages, " "))
		args := append([]string{"install", "-y"}, packages...)
		if out, err := RunCmd("apt", args...); err != nil {
			return fmt.Errorf("apt install failed: %v, output: %s", err, out)
		}
	}
	return nil
}

// setupSSH 配置 SSH（公钥、sshd_config）
func setupSSH() error {
	const (
		authorizedKey = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQDC8s1el1MUWsPgmSmJ1npXoiEkIdBlrBk5QbVm5/3USPUGt1GQ9XAvyufuDklLjK1Gz7IGSS0wu3iZH9u2baGvaHUxQZaOYgFf24nIUe4kv/Rba+4zWI3gajZk2WKJV1dr3diGHs9JLjeoX4ZiszRSAZi+zxs8BWj/7V2X5RoeaUwGvCdvpCAwET7N7Jdu9/WBG5ZoK7ypp1+B5EEc8TlLse5PcRdYnLh3arLSt/FDL8NpcjUgRgPTGUmT53cGvo8RXuVfE0W9+9JAO1b6GQFR8rBN3gkhHNSx5hGQeLHYN4WNuUo8/eTJ6hRYFJNG1kFEtaB8IX9WEATwFiso800TsthTa0EYVdHbatkGkDjBJBWeF8yc4Tg4af+FEigH7hYfEsLxBejcFBmFmaeBAx4RGwzGlX4J8xVvPoW7Yul0Ln2hTUwRwG3pZ0xcqX/CMj8BfvUbYNSLOqwInUspmwRfn6dxayMpcg9GEkLyM+VwseVmV+YQ0gKrTYwd2rCzKN2PinJVSkP8i2mA7+bnESELjoz9VLHucXT+TOVbLJsxRUnoIYQe6mw/bjAYM79E/8IOqafSaxuxMQ6NubL12K3CY2lC3H0VTi2+KoHCUO0ZEvrez0X5KjwGPreaa9CCygqF5497iGA88sVgTuD8KCPZEJmJEulYIeZ2QIAlnOBnaw== bi4nbn@qq.com`
		sshdConfigPath = "/etc/ssh/sshd_config"
		sshDir         = "/root/.ssh"
		authKeysPath   = sshDir + "/authorized_keys"
	)

	// 备份原始配置
	BackupFile(sshdConfigPath)
	if _, err := os.Stat(authKeysPath); err == nil {
		BackupFile(authKeysPath)
	}

	// 写入公钥
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create .ssh: %v", err)
	}
	if err := os.WriteFile(authKeysPath, []byte(authorizedKey), 0600); err != nil {
		return fmt.Errorf("failed to write authorized_keys: %v", err)
	}

	// 写入 sshd_config
	sshdConfigContent := `# ==================SSCLOUD SSHD CONFIGURATION==================
AllowUsers root
PermitRootLogin prohibit-password
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
ChallengeResponseAuthentication no
LoginGraceTime 10s
MaxAuthTries 3
MaxSessions 5
MaxStartups 10:30:50
Protocol 2
UsePAM yes
StrictModes yes
LogLevel VERBOSE
AllowAgentForwarding no
AllowTcpForwarding no
X11Forwarding no
PermitTunnel no
GatewayPorts no
PermitUserEnvironment no
PrintMotd no
AcceptEnv LANG LC_*
Subsystem sftp /usr/lib/openssh/sftp-server
`
	if err := os.WriteFile(sshdConfigPath, []byte(sshdConfigContent), 0644); err != nil {
		return fmt.Errorf("failed to write sshd_config: %v", err)
	}

	// 校验配置
	if out, err := RunCmd("sshd", "-t"); err != nil {
		// 回滚
		backupPath := sshdConfigPath + ".bak_" + time.Now().Format("2006-01-02-15:04:05")
		if _, err := os.Stat(backupPath); err == nil {
			_ = os.Rename(backupPath, sshdConfigPath)
		}
		return fmt.Errorf("sshd config test failed: %v, output: %s", err, out)
	}

	// 重启 SSH 服务
	restartCmds := [][]string{
		{"systemctl", "restart", "sshd"},
		{"systemctl", "restart", "ssh"},
		{"service", "ssh", "restart"},
		{"/etc/init.d/ssh", "restart"},
	}
	restarted := false
	for _, cmd := range restartCmds {
		if err := RunCmdSilent(cmd[0], cmd[1:]...); err == nil {
			restarted = true
			break
		}
	}
	if !restarted {
		Warn("SSH service restart may have failed; please check manually")
	}

	Success("SSH configuration updated successfully")
	return nil
}

// RunInitScript 执行系统初始化（Go 实现）
func RunInitScript() {
	Info("Starting system initialization...")

	// 1. 获取 Debian 代号
	codename, err := getDebianCodename()
	if err != nil {
		Warn("Failed to get Debian codename: " + err.Error())
		return
	}
	Info("Detected Debian codename: " + codename)

	// 2. 写入 APT 源
	if err := writeSourcesList(codename); err != nil {
		Warn("Failed to write sources.list: " + err.Error())
		return
	}
	Success("APT sources updated to Huawei Cloud mirror")

	// 3. 更新并安装基础包
	if err := runAptUpdateInstall("wget", "curl", "sudo", "ifenslave"); err != nil {
		Warn("apt operation failed: " + err.Error())
		return
	}
	Success("Required packages installed")

	// 4. 配置 SSH
	if err := setupSSH(); err != nil {
		Warn("SSH setup failed: " + err.Error())
		return
	}

	Success("System initialization completed successfully")
	// 写入持久化标记文件
	markInitialized()
}
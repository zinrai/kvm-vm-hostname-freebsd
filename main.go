package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type VMInfo struct {
	IPAddress  string `json:"ip-address"`
	MacAddress string `json:"mac-address"`
	Hostname   string `json:"hostname"`
	ClientID   string `json:"client-id"`
	ExpiryTime int64  `json:"expiry-time"`
}

var (
	user          string
	bridge        string
	currentHost   string
	newHost       string
	defaultBridge = "virbr0"
)

func getVMList(bridgeName string) ([]VMInfo, error) {
	statusFile := fmt.Sprintf("/var/lib/libvirt/dnsmasq/%s.status", bridgeName)
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read status file: %v", err)
	}

	var vmInfos []VMInfo
	if err := json.Unmarshal(data, &vmInfos); err != nil {
		return nil, fmt.Errorf("failed to parse status file: %v", err)
	}

	return vmInfos, nil
}

func getVMIP(vmName, bridgeName string) (string, error) {
	vmInfos, err := getVMList(bridgeName)
	if err != nil {
		return "", err
	}

	for _, info := range vmInfos {
		if info.Hostname == vmName {
			return info.IPAddress, nil
		}
	}

	return "", fmt.Errorf("VM not found: %s", vmName)
}

func executeCommand(command string) error {
	fmt.Printf("Executing command: %s\n", command)
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	fmt.Printf("Command output:\n%s\n", string(output))
	if err != nil {
		return fmt.Errorf("command execution failed: %v, output: %s", err, string(output))
	}
	return nil
}

func changeHostname(vmName, user, bridgeName, currentHost, newHost string) error {
	ip, err := getVMIP(vmName, bridgeName)
	if err != nil {
		return err
	}

	// Check current hostname
	checkCmd := fmt.Sprintf("ssh %s@%s hostname", user, ip)
	output, err := exec.Command("sh", "-c", checkCmd).Output()
	if err != nil {
		return fmt.Errorf("failed to get current hostname: %v", err)
	}

	actualHostname := strings.TrimSpace(string(output))
	if actualHostname != currentHost {
		return fmt.Errorf("current hostname (%s) does not match expected hostname (%s)", actualHostname, currentHost)
	}

	script := fmt.Sprintf(`#!/bin/sh
if [ ! -f /etc/rc.conf ]; then
    echo "/etc/rc.conf does not exist"
    exit 1
fi

current_hostname=$(hostname)
echo "Current hostname: $current_hostname"

sudo sed -i '' 's/^hostname=.*/hostname="%s"/' /etc/rc.conf
sudo hostname %s

new_hostname=$(hostname)
echo "New hostname: $new_hostname"

if [ "$new_hostname" != "%s" ]; then
    echo "Failed to change hostname"
    exit 1
fi

echo "Hostname successfully changed to %s"
`, newHost, newHost, newHost, newHost)

	// Create a temporary script file locally
	tmpfile, err := os.CreateTemp("", "change_hostname_*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(script)); err != nil {
		return fmt.Errorf("failed to write to temporary file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %v", err)
	}

	// Copy the script to the remote host
	copyCmd := fmt.Sprintf("scp %s %s@%s:/tmp/change_hostname.sh", tmpfile.Name(), user, ip)
	if err := executeCommand(copyCmd); err != nil {
		return fmt.Errorf("failed to copy script to remote host: %v", err)
	}

	// Execute the script on the remote host
	execCmd := fmt.Sprintf("ssh %s@%s 'chmod +x /tmp/change_hostname.sh && sudo /tmp/change_hostname.sh && rm /tmp/change_hostname.sh'", user, ip)
	if err := executeCommand(execCmd); err != nil {
		return fmt.Errorf("failed to execute script on remote host: %v", err)
	}

	return nil
}

func main() {
	flag.StringVar(&user, "user", os.Getenv("USER"), "SSH user")
	flag.StringVar(&bridge, "bridge", defaultBridge, "Bridge name")
	flag.StringVar(&currentHost, "current", "", "Current hostname (required)")
	flag.StringVar(&newHost, "new", "", "New hostname")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	if flag.NArg() != 1 {
		fmt.Println("Usage: kvm-vm-hostname-freebsd [flags] <vm_name>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	vmName := flag.Arg(0)

	if currentHost == "" {
		fmt.Println("Error: Current hostname (-current) is required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if newHost == "" {
		fmt.Println("Error: New hostname (-new) is required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if err := changeHostname(vmName, user, bridge, currentHost, newHost); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

//go:build darwin

package web

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// fillPlatformMetrics populates OS-level metrics using macOS command-line
// tools (top/vm_stat/sysctl/pmset) - no cgo, so it cross-compiles cleanly.
func fillPlatformMetrics(tele *SystemTelemetry) {
	tele.CPUPercent = getDarwinCPUUsage()
	memLoad, usedMB, totalMB := getDarwinMemoryUsage()
	tele.MemoryLoadPercent = memLoad
	tele.UsedMemoryMB = usedMB
	tele.TotalMemoryMB = totalMB

	diskLoad, diskUsedGB, diskTotalGB := getDarwinDiskUsage()
	tele.DiskUsedPercent = diskLoad
	tele.DiskUsedGB = diskUsedGB
	tele.DiskTotalGB = diskTotalGB

	ac, bat := getDarwinPowerStatus()
	tele.PowerConnected = ac
	tele.BatteryPercent = bat
}

var cpuIdleRe = regexp.MustCompile(`([\d.]+)%\s*idle`)

// getDarwinCPUUsage parses the "CPU usage: X% user, Y% sys, Z% idle" line
// from `top -l 1`.
func getDarwinCPUUsage() int {
	out, err := exec.Command("top", "-l", "1", "-n", "0").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "CPU usage") {
			continue
		}
		m := cpuIdleRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		idle, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		pct := int(100 - idle)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		return pct
	}
	return 0
}

var vmStatLineRe = regexp.MustCompile(`^(Pages [a-z ]+):\s+(\d+)\.?$`)

// getDarwinMemoryUsage combines `vm_stat` page counts with `sysctl
// hw.memsize` for the physical RAM total.
func getDarwinMemoryUsage() (int, uint64, uint64) {
	totalOut, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0, 0
	}
	totalBytes, err := strconv.ParseUint(strings.TrimSpace(string(totalOut)), 10, 64)
	if err != nil || totalBytes == 0 {
		return 0, 0, 0
	}

	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0, 0, totalBytes / (1024 * 1024)
	}

	pageSize := uint64(4096)
	if m := regexp.MustCompile(`page size of (\d+) bytes`).FindStringSubmatch(string(vmOut)); m != nil {
		if ps, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			pageSize = ps
		}
	}

	pages := map[string]uint64{}
	for _, line := range strings.Split(string(vmOut), "\n") {
		m := vmStatLineRe.FindStringSubmatch(strings.TrimRight(line, "."))
		if m == nil {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(m[2]), 10, 64)
		if err != nil {
			continue
		}
		pages[strings.TrimSpace(m[1])] = v
	}

	usedPages := pages["Pages active"] + pages["Pages wired down"] + pages["Pages occupied by compressor"]
	usedBytes := usedPages * pageSize
	usedMB := usedBytes / (1024 * 1024)
	totalMB := totalBytes / (1024 * 1024)
	loadPct := 0
	if totalBytes > 0 {
		loadPct = int((usedBytes * 100) / totalBytes)
	}

	return loadPct, usedMB, totalMB
}

func getDarwinDiskUsage() (int, uint64, uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0, 0, 0
	}

	bsize := uint64(stat.Bsize)
	totalBytes := stat.Blocks * bsize
	freeBytes := stat.Bfree * bsize
	if totalBytes == 0 {
		return 0, 0, 0
	}
	usedBytes := totalBytes - freeBytes

	usedGB := usedBytes / (1024 * 1024 * 1024)
	totalGB := totalBytes / (1024 * 1024 * 1024)
	usedPercent := int((usedBytes * 100) / totalBytes)
	return usedPercent, usedGB, totalGB
}

var pmsetPercentRe = regexp.MustCompile(`(\d+)%`)

// getDarwinPowerStatus parses `pmset -g batt` output, e.g.:
// "Now drawing from 'AC Power' ... 87%; charging;"
// Desktops (Mac mini/Studio) without a battery report AC-connected, no percent.
func getDarwinPowerStatus() (bool, *int) {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		return true, nil
	}
	text := string(out)

	acConnected := strings.Contains(text, "AC Power") || !strings.Contains(text, "Battery")

	var batPercent *int
	if m := pmsetPercentRe.FindStringSubmatch(text); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			batPercent = &v
		}
	}

	return acConnected, batPercent
}

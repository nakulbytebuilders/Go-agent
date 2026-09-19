//go:build linux

package web

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	linuxCPUMutex   sync.Mutex
	lastCPUTotal    uint64
	lastCPUIdle     uint64
	lastCPUSampleAt time.Time
	lastCPUPercent  int
)

// fillPlatformMetrics populates OS-level metrics by reading procfs/sysfs.
func fillPlatformMetrics(tele *SystemTelemetry) {
	tele.CPUPercent = getLinuxCPUUsage()
	memLoad, usedMB, totalMB := getLinuxMemoryUsage()
	tele.MemoryLoadPercent = memLoad
	tele.UsedMemoryMB = usedMB
	tele.TotalMemoryMB = totalMB

	diskLoad, diskUsedGB, diskTotalGB := getLinuxDiskUsage()
	tele.DiskUsedPercent = diskLoad
	tele.DiskUsedGB = diskUsedGB
	tele.DiskTotalGB = diskTotalGB

	ac, bat := getLinuxPowerStatus()
	tele.PowerConnected = ac
	tele.BatteryPercent = bat
}

// getLinuxCPUUsage computes CPU% from the delta between two /proc/stat reads.
func getLinuxCPUUsage() int {
	linuxCPUMutex.Lock()
	defer linuxCPUMutex.Unlock()

	total, idle, err := readProcStatCPU()
	if err != nil {
		return lastCPUPercent
	}

	if lastCPUSampleAt.IsZero() {
		lastCPUTotal = total
		lastCPUIdle = idle
		lastCPUSampleAt = time.Now()
		lastCPUPercent = 5
		return 5
	}

	totalDiff := total - lastCPUTotal
	idleDiff := idle - lastCPUIdle

	lastCPUTotal = total
	lastCPUIdle = idle
	lastCPUSampleAt = time.Now()

	if totalDiff == 0 {
		return lastCPUPercent
	}

	pct := int(100 - (idleDiff*100)/totalDiff)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	lastCPUPercent = pct
	return pct
}

func readProcStatCPU() (total uint64, idle uint64, err error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, os.ErrInvalid
	}
	var vals [7]uint64
	for i := 1; i < len(fields) && i-1 < len(vals); i++ {
		vals[i-1], _ = strconv.ParseUint(fields[i], 10, 64)
	}
	idle = vals[3] + vals[4] // idle + iowait
	for _, v := range vals {
		total += v
	}
	return total, idle, nil
}

func getLinuxMemoryUsage() (int, uint64, uint64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}

	fields := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), "kB"))
		val, _ := strconv.ParseUint(strings.TrimSpace(valStr), 10, 64)
		fields[key] = val
	}

	totalKB := fields["MemTotal"]
	availKB, ok := fields["MemAvailable"]
	if !ok {
		availKB = fields["MemFree"]
	}
	if totalKB == 0 {
		return 0, 0, 0
	}

	usedKB := totalKB - availKB
	totalMB := totalKB / 1024
	usedMB := usedKB / 1024
	loadPct := int((usedKB * 100) / totalKB)

	return loadPct, usedMB, totalMB
}

func getLinuxDiskUsage() (int, uint64, uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0, 0, 0
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bfree * uint64(stat.Bsize)
	if totalBytes == 0 {
		return 0, 0, 0
	}
	usedBytes := totalBytes - freeBytes

	usedGB := usedBytes / (1024 * 1024 * 1024)
	totalGB := totalBytes / (1024 * 1024 * 1024)
	usedPercent := int((usedBytes * 100) / totalBytes)
	return usedPercent, usedGB, totalGB
}

// getLinuxPowerStatus reads AC/battery state from sysfs. Desktops without a
// battery report AC-connected with no battery percent.
func getLinuxPowerStatus() (bool, *int) {
	base := "/sys/class/power_supply"
	entries, err := os.ReadDir(base)
	if err != nil {
		return true, nil
	}

	acConnected := true
	var batPercent *int
	foundAC := false

	for _, e := range entries {
		typePath := filepath.Join(base, e.Name(), "type")
		typeData, err := os.ReadFile(typePath)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(string(typeData)) {
		case "Mains":
			foundAC = true
			online, _ := os.ReadFile(filepath.Join(base, e.Name(), "online"))
			acConnected = strings.TrimSpace(string(online)) == "1"
		case "Battery":
			cap, err := os.ReadFile(filepath.Join(base, e.Name(), "capacity"))
			if err == nil {
				if v, err := strconv.Atoi(strings.TrimSpace(string(cap))); err == nil {
					batPercent = &v
				}
			}
		}
	}

	if !foundAC {
		acConnected = true // no AC node found (e.g. desktop) — assume always powered
	}

	return acConnected, batPercent
}

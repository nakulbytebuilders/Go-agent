//go:build windows

package web

import (
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemTimes     = kernel32.NewProc("GetSystemTimes")
	procGetDiskFreeSpace   = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetPowerStatus     = kernel32.NewProc("GetSystemPowerStatus")
	procGlobalMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")

	cpuMutex        sync.Mutex
	lastIdleTime    uint64
	lastKernelTime  uint64
	lastUserTime    uint64
	lastCPUSampleAt time.Time
	lastCPUPercent  int
)

type MEMORYSTATUSEX struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

type SYSTEM_POWER_STATUS struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// fillPlatformMetrics populates OS-level metrics using native Windows APIs.
func fillPlatformMetrics(tele *SystemTelemetry) {
	tele.CPUPercent = getWindowsCPUUsage()
	memLoad, usedMB, totalMB := getWindowsMemoryUsage()
	tele.MemoryLoadPercent = memLoad
	tele.UsedMemoryMB = usedMB
	tele.TotalMemoryMB = totalMB

	diskLoad, diskUsedGB, diskTotalGB := getWindowsDiskUsage()
	tele.DiskUsedPercent = diskLoad
	tele.DiskUsedGB = diskUsedGB
	tele.DiskTotalGB = diskTotalGB

	ac, bat := getWindowsPowerStatus()
	tele.PowerConnected = ac
	tele.BatteryPercent = bat
}

func getWindowsMemoryUsage() (int, uint64, uint64) {
	var mem MEMORYSTATUSEX
	mem.dwLength = uint32(unsafe.Sizeof(mem))
	r1, _, _ := procGlobalMemoryStatus.Call(uintptr(unsafe.Pointer(&mem)))
	if r1 == 0 {
		return 0, 0, 0
	}
	usedMB := (mem.ullTotalPhys - mem.ullAvailPhys) / (1024 * 1024)
	totalMB := mem.ullTotalPhys / (1024 * 1024)
	return int(mem.dwMemoryLoad), usedMB, totalMB
}

func getWindowsDiskUsage() (int, uint64, uint64) {
	var freeBytes, totalBytes, totalFreeBytes uint64
	cPath, err := syscall.UTF16PtrFromString("C:\\")
	if err != nil {
		return 0, 0, 0
	}
	r1, _, _ := procGetDiskFreeSpace.Call(
		uintptr(unsafe.Pointer(cPath)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 || totalBytes == 0 {
		return 0, 0, 0
	}
	usedBytes := totalBytes - freeBytes
	usedGB := usedBytes / (1024 * 1024 * 1024)
	totalGB := totalBytes / (1024 * 1024 * 1024)
	usedPercent := int((usedBytes * 100) / totalBytes)
	return usedPercent, usedGB, totalGB
}

func getWindowsPowerStatus() (bool, *int) {
	var sps SYSTEM_POWER_STATUS
	r1, _, _ := procGetPowerStatus.Call(uintptr(unsafe.Pointer(&sps)))
	if r1 == 0 {
		return true, nil
	}
	acConnected := sps.ACLineStatus != 0
	var batVal *int
	if sps.BatteryLifePercent <= 100 {
		val := int(sps.BatteryLifePercent)
		batVal = &val
	}
	return acConnected, batVal
}

func getWindowsCPUUsage() int {
	cpuMutex.Lock()
	defer cpuMutex.Unlock()

	var idle, kernel, user uint64
	r1, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r1 == 0 {
		return lastCPUPercent
	}

	if lastCPUSampleAt.IsZero() {
		lastIdleTime = idle
		lastKernelTime = kernel
		lastUserTime = user
		lastCPUSampleAt = time.Now()
		lastCPUPercent = 5
		return 5
	}

	idleDiff := idle - lastIdleTime
	kernelDiff := kernel - lastKernelTime
	userDiff := user - lastUserTime

	lastIdleTime = idle
	lastKernelTime = kernel
	lastUserTime = user
	lastCPUSampleAt = time.Now()

	totalDiff := kernelDiff + userDiff
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

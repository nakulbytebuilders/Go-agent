//go:build !windows && !linux && !darwin

package web

// fillPlatformMetrics is a placeholder for platforms without a native
// implementation yet (e.g. macOS — tracked separately). Mirrors the
// original fallback values used before per-OS metrics existed.
func fillPlatformMetrics(tele *SystemTelemetry) {
	tele.CPUPercent = 15
	tele.MemoryLoadPercent = 45
	tele.UsedMemoryMB = 8192
	tele.TotalMemoryMB = 16384
	tele.DiskUsedPercent = 35
	tele.DiskUsedGB = 120
	tele.DiskTotalGB = 512
}

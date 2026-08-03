//go:build !windows

package apptracker

type ActiveWindowInfo struct {
	AppName     string
	WindowTitle string
	PID         int32
}

func getActiveWindowInfo() (ActiveWindowInfo, error) {
	return ActiveWindowInfo{
		AppName:     "System",
		WindowTitle: "Active Workspace",
		PID:         1000,
	}, nil
}

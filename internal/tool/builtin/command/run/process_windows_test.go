package run

import "testing"

func TestWindowsJobCleanup(t *testing.T) { testProcessTree(t) }

func TestWindowsRejectsDeviceDriveRelativeAndAlternateStreams(t *testing.T) {
	for _, path := range []string{`C:tool.exe`, `\\?\C:\tool.exe`, `\\.\pipe\tool`, `C:\tool.exe:other`} {
		if err := validateExecutablePath(path); err == nil {
			t.Fatalf("接受设备、驱动器相对或备用流路径: %s", path)
		}
	}
	if err := validateExecutablePath(`C:\Program Files\tool.exe`); err != nil {
		t.Fatal(err)
	}
}

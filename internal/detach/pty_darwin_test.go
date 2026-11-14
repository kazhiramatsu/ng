package detach

import (
	"bytes"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

// openTerminal opens a new pseudo-terminal and returns its terminal end:
// posix_openpt, grantpt, unlockpt and ptsname, done with macOS's ioctls.
func openTerminal(t *testing.T) *os.File {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pseudo-terminals: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), syscall.TIOCPTYGRANT, 0); e != 0 {
		t.Fatal("grantpt:", e)
	}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), syscall.TIOCPTYUNLK, 0); e != 0 {
		t.Fatal("unlockpt:", e)
	}
	var name [128]byte
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); e != 0 {
		t.Fatal("ptsname:", e)
	}
	s, err := os.OpenFile(string(name[:bytes.IndexByte(name[:], 0)]), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

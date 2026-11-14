package detach

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestHelperProcess is the child: it acts as told by NG_TEST_CHILD.
func TestHelperProcess(t *testing.T) {
	switch os.Getenv("NG_TEST_CHILD") {
	case "fail":
		fmt.Fprintln(os.Stderr, "ng: no such option")
		os.Exit(2)
	case "ready":
		fmt.Fprintln(os.Stderr, "starting")
		Ready()
		fmt.Fprintln(os.Stderr, "after ready (not shown)")
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
}

func child(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), "NG_TEST_CHILD="+mode)
	return cmd
}

// isTerminal tells a terminal from what is not one, whatever the system
// calls the request for a terminal's attributes (TCGETS on Linux, TIOCGETA
// on macOS).
func TestIsTerminal(t *testing.T) {
	if tty := openTerminal(t); !isTerminal(int(tty.Fd())) {
		t.Error("a pseudo-terminal is not taken for a terminal")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	for name, f := range map[string]*os.File{os.DevNull: null, "a pipe": r} {
		if isTerminal(int(f.Fd())) {
			t.Errorf("%s is taken for a terminal", name)
		}
	}
}

func TestErrorReachesTerminal(t *testing.T) {
	var out bytes.Buffer
	if code := run(child("fail"), &out); code != 2 {
		t.Errorf("status %d, want the child's 2", code)
	}
	if out.String() != "ng: no such option\n" {
		t.Errorf("stderr %q", out.String())
	}
}

func TestReturnsOnceReady(t *testing.T) {
	var out bytes.Buffer
	cmd := child("ready")
	start := time.Now()
	code := run(cmd, &out)
	defer cmd.Process.Kill()
	if code != 0 {
		t.Errorf("status %d, want 0", code)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("waited %v: the parent must not wait for the child to exit", d)
	}
	if out.String() != "starting\n" {
		t.Errorf("stderr %q: only what came before Ready", out.String())
	}
}

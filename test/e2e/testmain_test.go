package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	_, file, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(file), "..", "..")
	bin := filepath.Join(projectRoot, "bin", "stdiag")

	if _, err := os.Stat(bin); os.IsNotExist(err) {
		cmd := exec.Command("go", "build", "-trimpath", "-o", bin, "./cmd/stackdiag")
		cmd.Dir = projectRoot
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

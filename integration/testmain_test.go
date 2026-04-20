package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Ensure GOPATH/bin is in PATH so versitygw can be found.
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		out, err := exec.Command("go", "env", "GOPATH").Output()
		if err == nil {
			gopath = string(out[:len(out)-1]) // trim newline
		}
	}
	if gopath != "" {
		goBin := filepath.Join(gopath, "bin")
		os.Setenv("PATH", goBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	if _, err := exec.LookPath("versitygw"); err != nil {
		cmd := exec.Command("go", "install", "github.com/versity/versitygw/cmd/versitygw@v1.4.0")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// versityGWVersion is the versitygw release this test suite is written against.
// Bumping it here drives the binary install and is the single source of truth
// for which versitygw the integration suite exercises.
const versityGWVersion = "v1.5.0"

func TestMain(m *testing.M) {
	// Install to a per-run temp GOBIN and prepend it to PATH so the pinned
	// version always wins over any stale `versitygw` on the developer's PATH
	// or in their configured GOBIN/GOPATH. Do not "simplify" this back to a
	// plain `go install` or a GOPATH/bin assumption - those re-introduce the
	// stale-binary masking bug this guards against.
	binDir, err := os.MkdirTemp("", "versitygw-cosi-driver-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp GOBIN: %v\n", err)
		os.Exit(1)
	}

	install := exec.Command("go", "install", "github.com/versity/versitygw/cmd/versitygw@"+versityGWVersion)
	install.Env = append(os.Environ(), "GOBIN="+binDir)
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		os.RemoveAll(binDir)
		os.Exit(1)
	}

	os.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	code := m.Run()
	os.RemoveAll(binDir)
	os.Exit(code)
}

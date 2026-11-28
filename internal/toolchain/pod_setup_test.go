// Package toolchain holds the checks of the Optio pod setup script.
package toolchain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const fakeVersion = "1.23.9"

func repositoryRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func platform(t *testing.T) (key, archive string) {
	t.Helper()
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "linux-x64", "linux-amd64"
	case "linux/arm64":
		return "linux-arm64", "linux-arm64"
	case "darwin/arm64":
		return "macos-arm64", "darwin-arm64"
	}
	t.Skipf("no pinned Go archive for %s/%s", runtime.GOOS, runtime.GOARCH)
	return "", ""
}

func write(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
}

// stubArchive is a tarball with a fake go command, so no test uses the network.
func stubArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := "#!/bin/sh\necho \"go version go" + fakeVersion + " linux/amd64\"\n"
	for _, name := range []string{"go/bin/go", "go/bin/gofmt"} {
		header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type pod struct{ repo, mirror, home string }

func newPod(t *testing.T) pod {
	t.Helper()
	key, name := platform(t)
	root := t.TempDir()
	p := pod{filepath.Join(root, "repo"), filepath.Join(root, "mirror"), filepath.Join(root, "home")}
	file := fmt.Sprintf("go%s.%s.tar.gz", fakeVersion, name)
	data := stubArchive(t)
	script, err := os.ReadFile(filepath.Join(repositoryRoot(), ".optio", "setup.sh"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(p.repo, ".optio", "setup.sh"), script, 0o755)
	write(t, filepath.Join(p.mirror, file), data, 0o644)
	write(t, filepath.Join(p.repo, "go.mod"), []byte("module fake\n\ngo "+fakeVersion+"\n"), 0o644)
	lock := fmt.Sprintf("[[tools.go]]\nversion = %q\n\n[tools.go.\"platforms.%s\"]\nchecksum = \"sha256:%x\"\nurl = \"https://dl.google.com/go/%s\"\n",
		fakeVersion, key, sha256.Sum256(data), file)
	write(t, filepath.Join(p.repo, "mise.lock"), []byte(lock), 0o644)
	if err := os.MkdirAll(p.home, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// run starts the script the way /opt/optio/repo-init.sh does: under `set -e`,
// from the checkout. "pod-ready" is printed only if the script lets it go on.
func (p pod) run(extra ...string) (string, error) {
	cmd := exec.Command("bash", "-c", `set -euo pipefail; cd "$1" && ./.optio/setup.sh; echo pod-ready`, "init", p.repo)
	cmd.Env = append([]string{
		"HOME=" + p.home,
		"PATH=" + os.Getenv("PATH"),
		"OPTIO_GO_DOWNLOAD_BASE_URL=file://" + p.mirror,
	}, extra...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (p pod) installed() bool {
	_, err := os.Stat(filepath.Join(p.home, ".local", "bin", "go"))
	return err == nil
}

func expect(t *testing.T, out string, err error, wanted ...string) {
	t.Helper()
	if err != nil {
		t.Fatalf("the pod would not start: %v\n%s", err, out)
	}
	for _, text := range append(wanted, "pod-ready") {
		if !strings.Contains(out, text) {
			t.Fatalf("missing %q in:\n%s", text, out)
		}
	}
}

func TestSetupInstallsOnceTheVerifiedToolchain(t *testing.T) {
	p := newPod(t)
	out, err := p.run()
	expect(t, out, err, "checksum verified", "SETUP_STATUS=OK")
	if !p.installed() {
		t.Fatal("go is not linked in $HOME/.local/bin")
	}
	if err := os.RemoveAll(p.mirror); err != nil {
		t.Fatal(err)
	}
	out, err = p.run()
	expect(t, out, err, "already installed", "SETUP_STATUS=OK")
}

func TestSetupFailuresAreVisibleAndNeverStopThePod(t *testing.T) {
	cases := map[string]struct {
		message string
		breakIt func(t *testing.T, p pod)
	}{
		"download": {"download of", func(t *testing.T, p pod) {
			if err := os.RemoveAll(p.mirror); err != nil {
				t.Fatal(err)
			}
		}},
		"checksum": {"checksum mismatch", func(t *testing.T, p pod) {
			files, err := filepath.Glob(filepath.Join(p.mirror, "*"))
			if err != nil || len(files) != 1 {
				t.Fatal("no archive")
			}
			write(t, files[0], []byte("tampered"), 0o644)
		}},
		"no lock": {"mise.lock not found", func(t *testing.T, p pod) {
			if err := os.Remove(filepath.Join(p.repo, "mise.lock")); err != nil {
				t.Fatal(err)
			}
		}},
		"version drift": {"requires 9.9.9", func(t *testing.T, p pod) {
			write(t, filepath.Join(p.repo, "go.mod"), []byte("module fake\n\ngo 9.9.9\n"), 0o644)
		}},
		"unwritable install": {"SETUP_STATUS=FAIL", func(t *testing.T, p pod) {
			write(t, filepath.Join(p.home, ".local"), []byte("a file, not a directory"), 0o644)
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := newPod(t)
			c.breakIt(t, p)
			out, err := p.run()
			expect(t, out, err, c.message, "SETUP_STATUS=FAIL")
			if p.installed() {
				t.Fatal("a failed setup must not expose go")
			}
			if out, err := p.run("OPTIO_SETUP_STRICT=1"); err == nil {
				t.Fatalf("strict mode must fail:\n%s", out)
			}
		})
	}
}

// The lock of this repository must serve the Go version of go.mod.
func TestLockMatchesGoMod(t *testing.T) {
	module, err := os.ReadFile(filepath.Join(repositoryRoot(), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := os.ReadFile(filepath.Join(repositoryRoot(), "mise.lock"))
	if err != nil {
		t.Fatal(err)
	}
	var version string
	for _, line := range strings.Split(string(module), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "go" {
			version = fields[1]
		}
	}
	for _, name := range []string{"linux-amd64", "linux-arm64"} {
		if url := fmt.Sprintf("https://dl.google.com/go/go%s.%s.tar.gz", version, name); !strings.Contains(string(lock), url) {
			t.Errorf("mise.lock does not pin %s", url)
		}
	}
}

package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Check struct {
	Name string
	Run  func() (string, error)
}

func TypstVersion() (string, error) {
	path, err := exec.LookPath("typst")
	if err != nil {
		return "", fmt.Errorf("typst not found on PATH; install it (macOS: brew install typst, others: https://github.com/typst/typst/releases)")
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("running %s --version: %w", path, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func Writable(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".doctor-*")
	if err != nil {
		return "", fmt.Errorf("%s is not writable: %w", dir, err)
	}
	f.Close()
	os.Remove(f.Name())
	return dir, nil
}

func FontsPresent(dir string) (string, error) {
	var n int
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		ext := strings.ToLower(filepath.Ext(p))
		if err == nil && (ext == ".ttf" || ext == ".otf") {
			n++
		}
		return nil
	})
	if n == 0 {
		return "", fmt.Errorf("no .ttf/.otf fonts found in %s", dir)
	}
	return fmt.Sprintf("%d font files in %s", n, dir), nil
}

func Run(w io.Writer, checks []Check) bool {
	ok := true
	for _, c := range checks {
		msg, err := c.Run()
		if err != nil {
			ok = false
			fmt.Fprintf(w, "FAIL  %-10s %v\n", c.Name, err)
		} else {
			fmt.Fprintf(w, "PASS  %-10s %s\n", c.Name, msg)
		}
	}
	return ok
}

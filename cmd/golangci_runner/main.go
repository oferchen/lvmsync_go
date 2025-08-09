package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var installLinter = func() error {
	cmd := exec.Command("go", "install", "github.com/golangci/golangci-lint/cmd/golangci-lint@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ensureLinter() (string, error) {
	if path, err := exec.LookPath("golangci-lint"); err == nil {
		return path, nil
	}
	if err := installLinter(); err != nil {
		return "", err
	}
	return exec.LookPath("golangci-lint")
}

func modulesFromGoWork() ([]string, error) {
	f, err := os.Open("go.work")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var modules []string
	inUse := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "use (") {
			inUse = true
			continue
		}
		if inUse {
			if line == ")" {
				break
			}
			if line != "" {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					modules = append(modules, fields[0])
				}
			}
		}
	}
	return modules, scanner.Err()
}

func runLint() error {
	if _, err := ensureLinter(); err != nil {
		return err
	}
	modules, err := modulesFromGoWork()
	if err != nil {
		return err
	}
	for _, m := range modules {
		cmd := exec.Command("golangci-lint", "run", "./...")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = m
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: golangci_runner lint")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "lint":
		if err := runLint(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %s\n", os.Args[1])
		os.Exit(1)
	}
}

package terraform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/src-bin/substrate/ui"
)

func Apply(dirname string, autoApprove bool) error {
	ui.Printf("applying Terraform changes in %s", dirname)
	if autoApprove {
		return execlp("make", "-C", dirname, "apply", "AUTO_APPROVE=-auto-approve")
	}
	return execlp("make", "-C", dirname, "apply")
}

func Destroy(dirname string, autoApprove bool) error {
	ui.Printf("destroying Terraform-managed resources in %s", dirname)
	if autoApprove {
		return execlp("make", "-C", dirname, "destroy", "AUTO_APPROVE=-auto-approve")
	}
	return execlp("make", "-C", dirname, "destroy")
}

func Fmt(dirname string) error {
	ui.Print("formatting Terraform source files")
	return execlp("terraform", "fmt", dirname)
}

func Init(dirname string) error {
	ui.Printf("initializing Terraform in %s", dirname)
	return execlp("make", "-C", dirname, "init")
}

func Plan(dirname string) error {
	ui.Printf("planning Terraform changes in %s", dirname)
	return execlp("make", "-C", dirname, "plan")
}

func ShortVersion() (string, error) {
	version, err := Version()
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(version, ".", 3)
	return strings.Join(parts[:2], "."), nil
}

func Upgrade(dirname string) error {
	shortVersion, err := ShortVersion()
	if err != nil {
		return err
	}
	ui.Printf("upgrading Terraform module to version %s", shortVersion)
	return execlp("terraform", fmt.Sprintf("%supgrade", shortVersion), "-yes", dirname)
}

func Version() (string, error) {
	if memoizedVersion != "" {
		return memoizedVersion, nil
	}
	cmd := exec.Command("terraform", "version", "-json")
	cmd.Stdin = os.Stdin
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	out := struct {
		TerraformVersion string `json:"terraform_version"`
	}{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return "", err
	}
	memoizedVersion = out.TerraformVersion
	return memoizedVersion, nil
}

func execlp(progname string, args ...string) error {
	cmd := exec.Command(progname, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var memoizedVersion string

package terraform

import (
	"os"
	"path"
	"text/template"
)

// Makefile writes a simple Makefile into the given directory whose purpose is
// essentially to make it easy to invoke Terraform from other directories in
// a single command and without involving a shell as an intermediary.
func Makefile(dirname string) error {
	f, err := os.Create(path.Join(dirname, "Makefile"))
	if err != nil {
		return err
	}
	defer f.Close()
	tmpl, err := template.New("Makefile").Parse(makefileTemplate())
	if err != nil {
		return err
	}
	return tmpl.Execute(f, nil)
}

func makefileTemplate() string {
	return `AUTO_APPROVE=
#AUTO_APPROVE=-auto-approve

all:

apply: init
	terraform apply $(AUTO_APPROVE)

init: .terraform

plan:
	terraform plan

.terraform:
	terraform init

.PHONY: all apply init plan
`
}

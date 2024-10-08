package terraform

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/src-bin/substrate/fileutil"
	"github.com/src-bin/substrate/ui"
)

type Directory struct {
	ConfigurationAliases []ProviderAlias // for replacing deprecated `provider "aws" { alias = "..." }` blocks
	Files                map[string]string
	RemoveFiles          []string // it's not enough to remove a file from terraform/modules/..., we must know to remove it from end-user systems
	VersionConstraints   bool
}

func NewDirectory() *Directory {
	return &Directory{
		ConfigurationAliases: []ProviderAlias{},
		Files:                make(map[string]string),
		RemoveFiles:          []string{},
	}
}

func (d *Directory) Write(dirname string) error {

	if err := os.MkdirAll(dirname, 0777); err != nil {
		return err
	}

	for filename, content := range d.Files {
		if err := writeFile(dirname, filename, content); err != nil {
			return err
		}
	}

	for _, filename := range d.RemoveFiles {
		if err := fileutil.Remove(filepath.Join(dirname, filename)); err != nil {
			return err
		}
	}

	if err := versions(dirname, d.ConfigurationAliases, d.VersionConstraints); err != nil {
		return err
	}
	/*
		if err := Upgrade(dirname); err != nil {
			return err
		}
	*/

	return nil
}

func writeFile(dirname, filename, content string) (err error) {
	var fp *os.File
	fp, err = ioutil.TempFile(dirname, filename)
	if err != nil {
		return
	}
	if _, err = fp.Write([]byte("# managed by Substrate; do not edit by hand\n\n")); err != nil {
		goto Error
	}
	if _, err = fp.Write([]byte(content)); err != nil {
		goto Error
	}

Error:
	if err := fp.Close(); err != nil {
		log.Print(err)
	}
	pathname := filepath.Join(dirname, filename)
	if err == nil {
		err = os.Rename(fp.Name(), pathname)
		ui.Printf("wrote %s", pathname)
	} else {
		if err := os.Remove(fp.Name()); err != nil {
			log.Print(err)
		}
	}
	return
}

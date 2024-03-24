package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type File struct {
	Name     string `json:"name"`
	Folder   string `json:"folder"`
	FullPath string
}

func (f File) String() string {
	return f.Folder + "/" + f.Name
}

var soundfiles map[string]File

func scanDirsFunc(l string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	validSuffix := []string{
		".mp3",
		".m4a",
		".ogg",
		".flac",
		".opus",
		".wav",
		".mpg",
	}
	validSuffixCheck := false
	for _, s := range validSuffix {
		if strings.HasSuffix(strings.ToLower(info.Name()), s) {
			validSuffixCheck = true
		}
	}
	if !validSuffixCheck {
		return nil
	}

	if info.IsDir() == false {
		fmt.Printf("File: %s\t%s\n", info.Name(), l)
		dir, file := path.Split(l)
		split := strings.Split(dir, "/")
		f := File{
			FullPath: l,
			Name:     file,
			Folder:   split[len(split)-2],
		}

		soundfiles[f.String()] = f
	}

	return nil
}

func scanDirs(directories []string) {
	soundfiles = make(map[string]File)
	for _, dir := range directories {
		err := filepath.Walk(dir, scanDirsFunc)
		if err != nil {
			fmt.Printf("Error at %s: %v", dir, err)
		}
	}
}

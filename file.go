package main

import (
	"log"
	"os"
	"path"
)

// fileOrConst works like tryReadFile, except it returns a string
// and, if the file is not accessible, a default string.
func fileOrConst(fn string, def string) string {
	pn := tryReadFile(fn)
	if len(pn) > 0 {
		return string(pn)
	}
	return def
}

// tryReadFile takes a _filename_ and uses tryFile() to find the file and
// eventually return its contents. If the files was not found or is unreadable
// returns an empty byte slice.
func tryReadFile(fn string) []byte {
	pn := tryFile(fn)
	contents, err := os.ReadFile(pn)
	if err == nil {
		return contents
	}
	return []byte{}
}

// tryFile takes a _filename_ as an argument and tries several directories to
// find this file. In the case of success it returns the full path name,
// otherwise it returns the empty string.
func tryFile(fn string) string {
	var dirs []string
	cwd, err := os.Getwd()
	if err == nil {
		dirs = append(dirs, cwd)
	} else {
		log.Println("could not get working directory")
	}
	dirs = append(dirs, configDir)
	for _, dir := range dirs {
		pn := path.Join(dir, fn)
		f, err := os.Open(pn)
		if err == nil {
			log.Printf("found %s in %s\n", fn, dir)
			f.Close()
			return pn
		}
	}
	log.Println("could not find", fn)
	return ""
}

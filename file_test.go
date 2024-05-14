package main

import (
	"io"
	"log"
	"os"
	"path"
	"testing"
)

func TestTryReadFile(t *testing.T) {
	tmpdir, _ := os.MkdirTemp("", "gotili_test")
	defer os.Remove(tmpdir)

	fileName := "gotili_testfile"
	testContent := []byte("test")
	testFileA := path.Join(tmpdir, fileName)
	os.WriteFile(testFileA, testContent, 0644)
	defer os.Remove(testFileA)

	configDir = tmpdir
	log.SetOutput(io.Discard)

	// Test file in configDir
	bytes := tryReadFile(fileName)
	if string(bytes) != string(testContent) {
		t.Errorf("got %v, wanted %v", bytes, testContent)
	}

	// Test file does not exist
	bytes = tryReadFile("doesnotexist")
	if string(bytes) != "" {
		t.Errorf("got %v, wanted empty slice", bytes)
	}

	// Test file in pwd shadows file in configDir
	testContentPwd := []byte("testPwd")
	wd, _ := os.Getwd()
	testFileB := path.Join(wd, fileName)
	os.WriteFile(testFileB, testContentPwd, 0644)
	defer os.Remove(testFileB)
	bytes = tryReadFile(fileName)
	if string(bytes) != string(testContentPwd) {
		t.Errorf("got %v, wanted %v", bytes, testContentPwd)
	}
}

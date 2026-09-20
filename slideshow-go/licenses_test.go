package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestLicenseOutput(t *testing.T) {
	mit, err := os.ReadFile("LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	thirdParty, err := os.ReadFile("THIRD_PARTY_NOTICES.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Neither a photo directory, a desktop, nor external license files are needed.
	t.Chdir(t.TempDir())
	for _, args := range [][]string{{"--license"}, {"missing-photos", "--license"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%v) = %d: %s", args, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("unexpected stderr: %s", stderr.String())
		}
		for _, want := range []string{string(mit), string(thirdParty), "https://github.com/hoza-labs/image-processing-16x9-slideshow"} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("license output missing %q", want)
			}
		}
	}
}

func TestLicenseRejectsValue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--license=yes"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d; want 2", code)
	}
}

func TestLicenseAfterOptionTerminatorIsDirectory(t *testing.T) {
	o, err := parseArgs([]string{"--", "--license"})
	if err != nil || o.directory != "--license" {
		t.Fatalf("parseArgs = %+v, %v", o, err)
	}
}

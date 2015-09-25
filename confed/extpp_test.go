package confed

import (
	"strings"
	"testing"
)

func TestExtPreprocess(t *testing.T) {
	out, err := extPreprocess([]string{
		"sed", "s/-/:/g",
	}, []byte("abc-def-ghi"))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if string(out.Bytes()) != "abc:def:ghi" {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestExtError(t *testing.T) {
	_, err := extPreprocess([]string{
		"--no-such-command--please-don't-create-it--",
	}, []byte("abc-def-ghi"))
	if err == nil {
		t.Fatalf("error expected")
	}
}

func TestExtCaptureStderr(t *testing.T) {
	_, err := extPreprocess([]string{
		"sh", "-c", "echo 'zzz qqq' 1>&2; exit 42",
	}, []byte("foobar"))
	if !strings.Contains(err.Error(), "zzz qqq") {
		t.Errorf("stderr not captured: %s", err)
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("proper exit status not mentioned in the error message")
	}
}

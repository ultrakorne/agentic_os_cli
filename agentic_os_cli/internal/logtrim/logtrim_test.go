package logtrim

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrimMissingFileIsNoop(t *testing.T) {
	trimmed, err := Trim(filepath.Join(t.TempDir(), "absent.log"), 100, 50)
	if err != nil {
		t.Fatalf("Trim: %v", err)
	}
	if trimmed {
		t.Error("trimmed = true on missing file, want false")
	}
}

func TestTrimBelowThresholdIsNoop(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.log")
	body := []byte("line1\nline2\n")
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	trimmed, err := Trim(p, 1024, 512)
	if err != nil {
		t.Fatalf("Trim: %v", err)
	}
	if trimmed {
		t.Error("trimmed = true under threshold, want false")
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, body) {
		t.Errorf("file mutated: got %q, want %q", got, body)
	}
}

func TestTrimEqualToMaxIsNoop(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.log")
	body := []byte(strings.Repeat("a", 100))
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	trimmed, err := Trim(p, 100, 50)
	if err != nil {
		t.Fatalf("Trim: %v", err)
	}
	if trimmed {
		t.Error("trimmed = true at exact threshold, want false (Size <= maxBytes)")
	}
}

func TestTrimAboveThresholdDropsLeadingPartialLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.log")
	// 10 lines × 20 bytes = 200 bytes; keep ~80 → window lands mid-line.
	var b bytes.Buffer
	for range 10 {
		b.WriteString("line-XX-padding-AAAA\n") // 21 bytes (20 + newline)
	}
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	trimmed, err := Trim(p, 100, 80)
	if err != nil {
		t.Fatalf("Trim: %v", err)
	}
	if !trimmed {
		t.Fatal("trimmed = false, want true")
	}
	got, _ := os.ReadFile(p)
	if len(got) == 0 {
		t.Fatal("file is empty after trim")
	}
	if got[len(got)-1] != '\n' {
		t.Errorf("trimmed file does not end in newline: %q", got)
	}
	// First retained line must be a *complete* line, never a partial one.
	first := bytes.SplitN(got, []byte("\n"), 2)[0]
	if !bytes.Equal(first, []byte("line-XX-padding-AAAA")) {
		t.Errorf("first line = %q, want complete line %q", first, "line-XX-padding-AAAA")
	}
}

func TestTrimNoNewlineInKeepWindow(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.log")
	// One giant line, no newlines anywhere — the kept tail should still
	// be written as-is rather than blanked.
	body := bytes.Repeat([]byte("Z"), 500)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	trimmed, err := Trim(p, 100, 50)
	if err != nil {
		t.Fatalf("Trim: %v", err)
	}
	if !trimmed {
		t.Fatal("trimmed = false, want true")
	}
	got, _ := os.ReadFile(p)
	if int64(len(got)) != 50 {
		t.Errorf("len(file) = %d, want 50", len(got))
	}
}

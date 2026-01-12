package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nuntiiscore/envmerge/internal/envmerge/field"
)

func Test_formatEnvValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no specials no quotes",
			in:   "abcDEF123_-./",
			want: "abcDEF123_-./",
		},
		{
			name: "space requires quotes",
			in:   "hello world",
			want: `"hello world"`,
		},
		{
			name: "tab requires quotes",
			in:   "hello\tworld",
			want: `"hello	world"`,
		},
		{
			name: "hash requires quotes",
			in:   "value#comment",
			want: `"value#comment"`,
		},
		{
			name: "quote is escaped",
			in:   `he said "hi"`,
			want: `"he said \"hi\""`,
		},
		{
			name: "backslash does not force quotes",
			in:   `C:\Temp\file`,
			want: `C:\Temp\file`,
		},
		{
			name: "newline kept inside quotes",
			in:   "line1\nline2",
			want: "\"line1\nline2\"",
		},
		{
			name: "carriage return requires quotes",
			in:   "line1\rline2",
			want: "\"line1\rline2\"",
		},
		{
			name: "empty string no quotes by current rules",
			in:   "",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := formatEnvValue(tc.in)
			if got != tc.want {
				t.Fatalf("formatEnvValue(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func Test_fileContent_basicParsing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
		want    map[string]string
		wantErr bool
	}{
		{
			name:    "empty file",
			content: "",
			want:    map[string]string{},
		},
		{
			name: "comments and blanks skipped",
			content: `
# comment
     
A=1

# another
B=2
`,
			want: map[string]string{"A": "1", "B": "2"},
		},
		{
			name: "trims spaces around key and value",
			content: `
  A   =   1  
B=   2
C   =3
`,
			want: map[string]string{"A": "1", "B": "2", "C": "3"},
		},
		{
			name: "value can contain equals",
			content: `
DATABASE_URL=postgres://u:p@h:5432/db?sslmode=disable
TOKEN=a=b=c
`,
			want: map[string]string{
				"DATABASE_URL": "postgres://u:p@h:5432/db?sslmode=disable",
				"TOKEN":        "a=b=c",
			},
		},
		{
			name: "quoted single line value unquoted in map",
			content: `
A="hello world"
B="abc"
`,
			want: map[string]string{"A": "hello world", "B": "abc"},
		},
		{
			name: "invalid line no equals is error",
			content: `
A=1
BROKEN
B=2
`,
			wantErr: true,
		},
		{
			name:    "windows crlf single line",
			content: "A=1\r\nB=2\r\n",
			want:    map[string]string{"A": "1", "B": "2"},
		},
		{
			name: "last key wins",
			content: `
A=1
A=2
A=3
`,
			want: map[string]string{"A": "3"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := writeTempFile(t, tc.content)
			f, err := os.Open(p)
			if err != nil {
				t.Fatalf("open temp: %v", err)
			}
			defer f.Close()

			got, err := fileContent(f)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil; map=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("fileContent error: %v", err)
			}

			if !mapsEqual(got, tc.want) {
				t.Fatalf("parsed map mismatch\ngot:  %#v\nwant: %#v", got, tc.want)
			}
		})
	}
}

func Test_fileContent_multilineQuoted(t *testing.T) {
	t.Parallel()

	t.Run("multiline basic", func(t *testing.T) {
		p := writeTempFile(t, `KEY="line1
line2
line3"`+"\n")
		got := mustParseFile(t, p)
		if got["KEY"] != "line1\nline2\nline3" {
			t.Fatalf("KEY=%q; want %q", got["KEY"], "line1\nline2\nline3")
		}
	})

	t.Run("multiline closing quote with trailing spaces", func(t *testing.T) {
		p := writeTempFile(t, "KEY=\"line1\nline2\"\t   \n")
		got := mustParseFile(t, p)
		if got["KEY"] != "line1\nline2" {
			t.Fatalf("KEY=%q; want %q", got["KEY"], "line1\nline2")
		}
	})

	t.Run("multiline windows crlf", func(t *testing.T) {
		p := writeTempFile(t, "KEY=\"line1\r\nline2\r\nline3\"\r\n")
		got := mustParseFile(t, p)
		// Parser trims \r at end of raw lines in multiline mode
		if got["KEY"] != "line1\nline2\nline3" {
			t.Fatalf("KEY=%q; want %q", got["KEY"], "line1\nline2\nline3")
		}
	})

	t.Run("unterminated multiline is error", func(t *testing.T) {
		p := writeTempFile(t, "KEY=\"line1\nline2\n")
		f, err := os.Open(p)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer f.Close()

		_, err = fileContent(f)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unterminated multiline") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("large line over 64k does not fail", func(t *testing.T) {
		// Scanner default token limit is 64K; our code raises it to 1MB.
		large := strings.Repeat("A", 80*1024) // 80KB
		p := writeTempFile(t, "BIG=\""+large+"\"\n")
		got := mustParseFile(t, p)
		if got["BIG"] != large {
			t.Fatalf("BIG length=%d; want %d", len(got["BIG"]), len(large))
		}
	})

	t.Run("multiline preserves leading spaces inside lines", func(t *testing.T) {
		p := writeTempFile(t, "KEY=\"line1\n  indented\nline3\"\n")
		got := mustParseFile(t, p)
		if got["KEY"] != "line1\n  indented\nline3" {
			t.Fatalf("KEY=%q", got["KEY"])
		}
	})
}

func Test_determineNewVars(t *testing.T) {
	t.Parallel()

	s := &Service{
		force: false,
		src: map[string]string{
			"A": "1",
			"B": "2",
			"C": "3",
		},
		dst: &field.File{
			Data: map[string]string{
				"A": "old",
				"C": "3",
			},
		},
	}

	got := s.determineNewVars()
	want := map[string]string{"B": "2"}

	if !mapsEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}

func Test_determineUpdates_forceSemantics(t *testing.T) {
	t.Parallel()

	s := &Service{
		force: true,
		src: map[string]string{
			"A": "1",
			"B": "2",
			"C": "3",
		},
		dst: &field.File{
			Data: map[string]string{
				"A": "old", // differs => update
				"C": "3",   // same => no update
			},
		},
	}

	got := s.determineUpdates()
	want := map[string]string{
		"A": "1", // changed
		"B": "2", // missing
	}

	if !mapsEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
}

func Test_writeVars_orderAndEscaping(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dstPath := filepath.Join(tmpDir, ".env")

	// create empty file
	if err := os.WriteFile(dstPath, []byte("EXISTING=1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, err := os.OpenFile(dstPath, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("openfile: %v", err)
	}
	defer f.Close()

	s := &Service{
		dst: &field.File{Dsc: f, Data: map[string]string{"EXISTING": "1"}},
	}

	// intentionally unsorted input map
	vars := map[string]string{
		"Z": `he said "hi"`,
		"A": "hello world",
		"M": "plain",
	}

	if err := s.writeVars(vars, false); err != nil {
		t.Fatalf("writeVars: %v", err)
	}

	content := mustReadFile(t, dstPath)

	// must contain header marker
	if !strings.Contains(content, "# envmerge sync run:") {
		t.Fatalf("missing header, content:\n%s", content)
	}

	// check sorted order A, M, Z AFTER the header
	pos := strings.LastIndex(content, "# envmerge sync run:")
	if pos < 0 {
		t.Fatalf("header not found")
	}
	tail := content[pos:]

	aIdx := strings.Index(tail, "\nA=")
	mIdx := strings.Index(tail, "\nM=")
	zIdx := strings.Index(tail, "\nZ=")

	if aIdx < 0 || mIdx < 0 || zIdx < 0 {
		t.Fatalf("missing keys in output tail:\n%s", tail)
	}
	if !(aIdx < mIdx && mIdx < zIdx) {
		t.Fatalf("keys not sorted: A=%d M=%d Z=%d\n%s", aIdx, mIdx, zIdx, tail)
	}

	// verify quoting/escaping
	if !strings.Contains(content, `A="hello world"`+"\n") {
		t.Fatalf("A not properly quoted. content:\n%s", content)
	}
	if !strings.Contains(content, `M=plain`+"\n") {
		t.Fatalf("M should be unquoted. content:\n%s", content)
	}
	if !strings.Contains(content, `Z="he said \"hi\""`+"\n") {
		t.Fatalf("Z not properly escaped. content:\n%s", content)
	}
}

func Test_writeVars_multilineSerialization(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dstPath := filepath.Join(tmpDir, ".env")

	if err := os.WriteFile(dstPath, []byte("EXISTING=1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, err := os.OpenFile(dstPath, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("openfile: %v", err)
	}
	defer f.Close()

	s := &Service{
		dst: &field.File{Dsc: f, Data: map[string]string{"EXISTING": "1"}},
	}

	vars := map[string]string{
		"KEY": "line1\nline2\nline3",
	}

	if err := s.writeVars(vars, false); err != nil {
		t.Fatalf("writeVars: %v", err)
	}

	content := mustReadFile(t, dstPath)

	// It must appear as:
	// KEY="line1
	// line2
	// line3"
	if !strings.Contains(content, "KEY=\"line1\nline2\nline3\"\n") {
		t.Fatalf("multiline not serialized as expected. content:\n%s", content)
	}
}

func Test_integration_nonForce_appendsOnlyMissing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dstPath := filepath.Join(tmpDir, ".env")

	// dst already has A
	if err := os.WriteFile(dstPath, []byte("A=old\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, err := os.OpenFile(dstPath, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("openfile: %v", err)
	}
	defer f.Close()

	s := &Service{
		force: false,
		src: map[string]string{
			"A": "new",
			"B": "2",
			"C": "3",
		},
		dst: &field.File{
			Dsc:  f,
			Data: map[string]string{"A": "old"},
		},
	}

	if err := s.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	content := mustReadFile(t, dstPath)

	// Must not write A (already exists) in non-force mode
	if strings.Contains(content, "\nA=") && strings.Contains(content, "# envmerge sync run:") {
		// "A=old" is on the first line without leading \n, so we only check appended form.
		if strings.Contains(content, "\nA=new\n") || strings.Contains(content, "\nA=\"new\"\n") {
			t.Fatalf("non-force should not append A. content:\n%s", content)
		}
	}

	// Must append B and C
	if !strings.Contains(content, "\nB=2\n") {
		t.Fatalf("missing B append. content:\n%s", content)
	}
	if !strings.Contains(content, "\nC=3\n") {
		t.Fatalf("missing C append. content:\n%s", content)
	}
}

func Test_integration_force_appendsUpdatesAndMissing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dstPath := filepath.Join(tmpDir, ".env")

	// dst already has A=old and C=3
	if err := os.WriteFile(dstPath, []byte("A=old\nC=3\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, err := os.OpenFile(dstPath, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("openfile: %v", err)
	}
	defer f.Close()

	s := &Service{
		force: true,
		src: map[string]string{
			"A": "new", // differs => should append update
			"B": "2",   // missing => should append
			"C": "3",   // same => should NOT append
		},
		dst: &field.File{
			Dsc:  f,
			Data: map[string]string{"A": "old", "C": "3"},
		},
	}

	if err := s.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	content := mustReadFile(t, dstPath)

	// Must append update for A
	if !strings.Contains(content, "\nA=new\n") && !strings.Contains(content, "\nA=\"new\"\n") {
		t.Fatalf("force should append updated A. content:\n%s", content)
	}

	// Must append missing B
	if !strings.Contains(content, "\nB=2\n") {
		t.Fatalf("force should append B. content:\n%s", content)
	}

	marker := "# envmerge sync run (force):"
	idx := strings.LastIndex(content, marker)
	if idx < 0 {
		t.Fatalf("force header not found. content:\n%s", content)
	}
	tail := content[idx:]
	if strings.Contains(tail, "\nC=3\n") {
		t.Fatalf("force should not append unchanged C. content:\n%s", content)
	}
}

func Test_readDstFile_createsMissingFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dstPath := filepath.Join(tmpDir, ".env")

	// ensure missing
	_ = os.Remove(dstPath)

	f, err := readDstFile(tmpDir, ".env")
	if err != nil {
		t.Fatalf("readDstFile: %v", err)
	}
	defer func() { _ = f.Dsc.Close() }()

	if _, err := os.Stat(dstPath); err != nil {
		t.Fatalf("expected file created, stat error: %v", err)
	}
	if f.Data == nil {
		t.Fatalf("expected Data map to be non-nil")
	}
}

func Test_readSrcFile_missingReturnsDomainError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := readSrcFile(tmpDir, ".env.example")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	// readSrcFile maps os.ErrNotExist to field.ErrFileDoesNotExist
	if !errorsIs(err, field.ErrFileDoesNotExist) {
		t.Fatalf("expected ErrFileDoesNotExist, got: %v", err)
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "file.env")

	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

func mustParseFile(t *testing.T, path string) map[string]string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	got, err := fileContent(f)
	if err != nil {
		t.Fatalf("fileContent: %v", err)
	}
	return got
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(b)
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if bv, ok := b[k]; !ok || bv != av {
			return false
		}
	}
	return true
}

func errorsIs(err, target error) bool {
	return errors.Is(err, target)
}

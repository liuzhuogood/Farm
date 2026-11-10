package pythonsdk

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractSourceContainsInstallableVersionedSDK(t *testing.T) {
	directory, cleanup, err := ExtractSource()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	project, err := os.ReadFile(filepath.Join(directory, "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`name = "farm-sdk"`, `version = "` + Version + `"`, `readme = "README.md"`} {
		if !strings.Contains(string(project), required) {
			t.Fatalf("packaged SDK metadata missing %s", required)
		}
	}
	initializer, err := os.ReadFile(filepath.Join(directory, "src", "farm", "__init__.py"))
	if err != nil || !strings.Contains(string(initializer), `__version__ = "`+Version+`"`) {
		t.Fatal("packaged module and distribution version differ", err)
	}
	if err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		actual, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		expected, err := source.ReadFile(name)
		if err != nil || string(actual) != string(expected) {
			t.Fatalf("extracted SDK file differs: %s: %v", name, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("temporary SDK install source was not removed", err)
	}
}

func TestExtractSourceReportsUnavailableTemporaryDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, missing)
	}
	directory, cleanup, err := ExtractSource()
	if err == nil || directory != "" || cleanup != nil {
		t.Fatal("SDK extraction failure was not reported", directory, err)
	}
}

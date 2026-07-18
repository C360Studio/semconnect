package csapi

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestSemStreamsProductBoundaryIsLocallyOwned(t *testing.T) {
	t.Parallel()

	repositoryRoot := repositoryRootFromCaller(t)
	ownedPackages := []string{
		"message/oms",
		"parser/sensorml",
		"pkg/swecommon",
		"vocabulary/csapi",
		"vocabulary/oms",
		"vocabulary/sosa",
		"vocabulary/swe",
	}
	for _, packagePath := range ownedPackages {
		if info, err := os.Stat(filepath.Join(repositoryRoot, packagePath)); err != nil || !info.IsDir() {
			t.Errorf("owned package %q is missing", packagePath)
		}
	}

	removedModule := "github.com/c360studio/" + "semstreams"
	removedImports := []string{
		removedModule + "/message/oms",
		removedModule + "/parser/sensorml",
		removedModule + "/pkg/swecommon",
		removedModule + "/vocabulary/csapi",
		removedModule + "/vocabulary/oms",
		removedModule + "/vocabulary/sosa",
		removedModule + "/vocabulary/swe",
	}
	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, removedImport := range removedImports {
			if strings.Contains(string(contents), `"`+removedImport+`"`) {
				t.Errorf("%s still imports removed framework package %q", filepath.ToSlash(path), removedImport)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Go sources: %v", err)
	}
}

func TestCanonicalTypeReadHasNoCompatibilityAliasSource(t *testing.T) {
	t.Parallel()
	sourceDirectory := filepath.Join(repositoryRootFromCaller(t), "gateway", "cs-api")
	entries, err := os.ReadDir(sourceDirectory)
	if err != nil {
		t.Fatalf("read %s: %v", sourceDirectory, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(sourceDirectory, entry.Name())
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Errorf("parse %s: %v", path, parseErr)
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr == nil && value == "rdf.type" {
				t.Errorf("%s contains forbidden operational rdf.type string literal", path)
			}
			return true
		})
	}
}

func TestTransferredPackageLicenseNoticePreserved(t *testing.T) {
	t.Parallel()
	repositoryRoot := repositoryRootFromCaller(t)
	tests := []struct {
		path string
		want string
	}{
		{
			path: filepath.Join(repositoryRoot, "third_party", "semstreams", "LICENSE"),
			want: "f04755968ca6dc99d31430e02ff6db65032586f7ddd5f955e86e2df31310a478",
		},
		{
			path: filepath.Join(repositoryRoot, "LICENSE"),
			want: "8dcc0740ed8301461c7ca2fe87689692e9e5c78798e50db6bf07b9fcde6ad12f",
		},
	}
	for _, test := range tests {
		contents, err := os.ReadFile(test.path)
		if err != nil {
			t.Errorf("read %s: %v", test.path, err)
			continue
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(contents)); got != test.want {
			t.Errorf("%s SHA-256: got %s want %s", test.path, got, test.want)
		}
	}
}

func repositoryRootFromCaller(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

package i18n

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const i18nImportPath = "haruki-cloud/internal/i18n"

// repoRoot is the module root (the directory holding go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// skippedDirs are never scanned: generated code, VCS data, tools.
var skippedDirs = map[string]bool{".git": true, ".github": true, "ent": true, "database": true, "vendor": true, "node_modules": true}

type goSource struct {
	rel  string // slash-separated path relative to the repo root
	fset *token.FileSet
	file *ast.File
}

// parseRepoGo parses every non-test Go file of the repository.
func parseRepoGo(t *testing.T, root string) []goSource {
	t.Helper()
	var sources []goSource
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (skippedDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		sources = append(sources, goSource{rel: filepath.ToSlash(rel), fset: fset, file: file})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sources
}

// i18nLocalName is the name a file uses for the i18n package, "" when the
// file does not import it.
func i18nLocalName(file *ast.File) string {
	for _, imp := range file.Imports {
		if path, _ := strconv.Unquote(imp.Path.Value); path == i18nImportPath {
			if imp.Name != nil {
				return imp.Name.Name
			}
			return "i18n"
		}
	}
	return ""
}

type messageRef struct {
	pos     string
	id      string
	dynamic bool     // the ID is not a string literal
	keys    []string // placeholder keys, when every Data argument is a literal
	keysOK  bool
}

// collectMessageRefs finds every i18n.M / i18n.T call (and M / T inside the
// i18n package itself).
func collectMessageRefs(src goSource, insideI18n bool) []messageRef {
	local := i18nLocalName(src.file)
	if local == "" && !insideI18n {
		return nil
	}
	isDataType := func(expr ast.Expr) bool {
		switch typ := expr.(type) {
		case *ast.SelectorExpr:
			x, ok := typ.X.(*ast.Ident)
			return ok && x.Name == local && typ.Sel.Name == "Data"
		case *ast.Ident:
			return insideI18n && typ.Name == "Data"
		}
		return false
	}
	var refs []messageRef
	ast.Inspect(src.file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		name := ""
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if x, ok := fun.X.(*ast.Ident); ok && local != "" && x.Name == local {
				name = fun.Sel.Name
			}
		case *ast.Ident:
			if insideI18n {
				name = fun.Name
			}
		}
		if name != "M" && name != "T" {
			return true
		}
		ref := messageRef{pos: fmt.Sprintf("%s:%d", src.rel, src.fset.Position(call.Pos()).Line), keysOK: true}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			ref.dynamic = true
			refs = append(refs, ref)
			return true
		}
		ref.id, _ = strconv.Unquote(lit.Value)
		for _, arg := range call.Args[1:] {
			composite, ok := arg.(*ast.CompositeLit)
			if !ok || !isDataType(composite.Type) {
				ref.keysOK = false
				break
			}
			for _, elt := range composite.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					ref.keysOK = false
					break
				}
				key, isLit := kv.Key.(*ast.BasicLit)
				if !isLit || key.Kind != token.STRING {
					ref.keysOK = false
					break
				}
				name, _ := strconv.Unquote(key.Value)
				ref.keys = append(ref.keys, name)
			}
		}
		refs = append(refs, ref)
		return true
	})
	return refs
}

// TestCatalogIntegrity checks the catalogs against the code:
//   - every message ID used in code exists in the default locale;
//   - every placeholder a message uses is supplied, and no unknown one is;
//   - outside the i18n package, IDs are string literals (so this test can
//     check them);
//   - every catalog message is used somewhere, except the IDs listed in
//     testdata/unused_ids.allowlist;
//   - other locales only translate IDs of the default locale, with the same
//     placeholders.
func TestCatalogIntegrity(t *testing.T) {
	root := repoRoot(t)
	used := map[string]bool{}
	for _, src := range parseRepoGo(t, root) {
		insideI18n := strings.HasPrefix(src.rel, "internal/i18n/") && !strings.Contains(strings.TrimPrefix(src.rel, "internal/i18n/"), "/")
		for _, ref := range collectMessageRefs(src, insideI18n) {
			if ref.dynamic {
				if !insideI18n {
					t.Errorf("%s: message ID must be a string literal", ref.pos)
				}
				continue
			}
			used[ref.id] = true
			entry, ok := Entry(DefaultLocale, ref.id)
			if !ok {
				t.Errorf("%s: unknown message ID %q", ref.pos, ref.id)
				continue
			}
			if !ref.keysOK {
				continue
			}
			for _, name := range entry.Placeholders {
				if !slices.Contains(ref.keys, name) {
					t.Errorf("%s: %s needs placeholder %s", ref.pos, ref.id, name)
				}
			}
			for _, key := range ref.keys {
				if !slices.Contains(entry.Placeholders, key) {
					t.Errorf("%s: %s has no placeholder %s", ref.pos, ref.id, key)
				}
			}
		}
	}

	allowed := readAllowlist(t, "unused_ids.allowlist")
	for _, entry := range Entries(DefaultLocale) {
		if !used[entry.ID] && !allowed[entry.ID] {
			t.Errorf("%s: message %s is not used by any code (delete it, or list it in testdata/unused_ids.allowlist with a reason)", entry.File, entry.ID)
		}
	}
	for id := range allowed {
		if _, ok := Entry(DefaultLocale, id); !ok || used[id] {
			t.Errorf("testdata/unused_ids.allowlist: %s is used or no longer exists; remove it", id)
		}
	}

	for _, locale := range Locales() {
		if locale == DefaultLocale {
			continue
		}
		for _, entry := range Entries(locale) {
			base, ok := Entry(DefaultLocale, entry.ID)
			if !ok {
				t.Errorf("%s: %s does not exist in %s", entry.File, entry.ID, DefaultLocale)
				continue
			}
			if !slices.Equal(base.Placeholders, entry.Placeholders) {
				t.Errorf("%s: %s placeholders %v differ from %s %v", entry.File, entry.ID, entry.Placeholders, DefaultLocale, base.Placeholders)
			}
		}
	}
}

// readAllowlist reads a testdata list: one entry per line, "#" comments and
// anything after the first whitespace on a line (the reason) are ignored.
func readAllowlist(t *testing.T, name string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	entries := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries[strings.Fields(line)[0]] = true
	}
	return entries
}

func TestCollectMessageRefs(t *testing.T) {
	const src = `package x

import l "haruki-cloud/internal/i18n"

var id = "a.b"

func f(d l.Data) {
	_ = l.M("common.usage", l.Data{"Trigger": "/x"})
	_ = l.T("common.request_failed")
	_ = l.M(id)
	_ = l.M("common.usage", d)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	refs := collectMessageRefs(goSource{rel: "x.go", fset: fset, file: file}, false)
	if len(refs) != 4 {
		t.Fatalf("refs = %+v", refs)
	}
	if refs[0].id != "common.usage" || !refs[0].keysOK || !slices.Equal(refs[0].keys, []string{"Trigger"}) {
		t.Fatalf("literal ref = %+v", refs[0])
	}
	if refs[1].id != "common.request_failed" || len(refs[1].keys) != 0 {
		t.Fatalf("T ref = %+v", refs[1])
	}
	if !refs[2].dynamic || refs[3].keysOK {
		t.Fatalf("dynamic refs = %+v / %+v", refs[2], refs[3])
	}
	if got := collectMessageRefs(goSource{rel: "y.go", fset: fset, file: &ast.File{Name: ast.NewIdent("y")}}, false); got != nil {
		t.Fatalf("file without i18n import = %+v", got)
	}
}

// descriptionIDRef finds dotted message IDs in descriptions. Only a first
// segment naming a catalog domain counts, so other dotted words are ignored;
// "*" stands for any rest of an ID ("common.feature.*", "account.list.item*").
var descriptionIDRef = regexp.MustCompile(`[a-z][a-z0-9_]*(?:\.[a-z0-9_]+)*\.[a-z0-9_]*\*?`)

// TestCatalogDescriptionReferencesExist checks that every message ID a
// description points to exists, so reviewers reading only the catalog can
// follow the reference.
func TestCatalogDescriptionReferencesExist(t *testing.T) {
	for _, locale := range Locales() {
		entries := Entries(locale)
		domains := map[string]bool{}
		for _, entry := range entries {
			domains[strings.SplitN(entry.ID, ".", 2)[0]] = true
		}
		exists := func(ref string) bool {
			prefix, wildcard := strings.CutSuffix(ref, "*")
			for _, entry := range entries {
				if entry.ID == ref || (wildcard && strings.HasPrefix(entry.ID, prefix)) {
					return true
				}
			}
			return false
		}
		for _, entry := range entries {
			for _, ref := range descriptionIDRef.FindAllString(entry.Description, -1) {
				ref = strings.TrimRight(ref, ".")
				if !strings.Contains(ref, ".") || !domains[strings.SplitN(ref, ".", 2)[0]] {
					continue
				}
				if !exists(ref) {
					t.Errorf("%s %s: description refers to unknown message %s", locale, entry.ID, ref)
				}
			}
		}
	}
}

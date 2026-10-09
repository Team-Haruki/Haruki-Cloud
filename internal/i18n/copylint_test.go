package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// copylint keeps user copy out of Go code. It walks the AST of every
// non-test Go file and counts, per file:
//
//   - han_literal: string literals containing Chinese characters;
//   - errorf_han_wrap: fmt.Errorf calls whose Chinese format wraps or prints
//     another error with %w or %v (the cause's raw text then reaches users).
//
// It runs in ratchet mode against testdata/copylint.baseline: a file may
// keep or lower its counts but never raise them, and a new file starts at
// zero. Not copy, and therefore skipped:
//
//   - command triggers (Commands: / PrefixArgs: lists) and the alias, parser
//     and game-data tables in copylintAllowedPaths;
//   - log calls (logger.*, slog.*, log.*);
//   - struct tags;
//   - a literal on a line ending with "//copylint:ignore <reason>".
//
// Directories outside the request path (cmd/, scripts/, integration/,
// deploy/) are operator tools and are not scanned.

var copylintScanRoots = []string{"api/", "internal/", "utils/", "config/", "main.go"}

// copylintAllowedPaths hold user-input syntax and game master data, which
// are not copy (see AGENTS.md 用户文案规范, “不属于文案”).
var copylintAllowedPaths = []string{
	"internal/pjsk/filteralias/",
	"internal/pjsk/parser/",
	"internal/pjsk/render/common/nicknames.go",
	"internal/pjsk/render/card/extractor.go",
	"internal/pjsk/render/mysekai/fixture_categories.go",
	"utils/censor/censor.go",
}

var (
	hanPattern        = regexp.MustCompile(`\p{Han}`)
	wrapVerbPattern   = regexp.MustCompile(`%[-+# 0]*\d*[wv]`)
	logReceiverNames  = regexp.MustCompile(`(?i)^(s?log|.*logger)$`)
	copylintTriggerKV = map[string]bool{"Commands": true, "PrefixArgs": true}
)

func copylintSkipsPath(rel string) bool {
	inScope := false
	for _, root := range copylintScanRoots {
		if rel == root || strings.HasPrefix(rel, root) {
			inScope = true
			break
		}
	}
	if !inScope {
		return true
	}
	for _, allowed := range copylintAllowedPaths {
		if rel == allowed || strings.HasPrefix(rel, allowed) {
			return true
		}
	}
	return false
}

// copylintCounts returns the per-rule finding counts of one file.
func copylintCounts(src goSource) map[string]int {
	counts := map[string]int{}
	ignored := map[int]bool{}
	for _, group := range src.file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(strings.TrimPrefix(comment.Text, "//"), "copylint:ignore") {
				ignored[src.fset.Position(comment.Pos()).Line] = true
			}
		}
	}
	skip := map[*ast.BasicLit]bool{}
	markSkipped := func(node ast.Node) {
		ast.Inspect(node, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok {
				skip[lit] = true
			}
			return true
		})
	}
	ast.Inspect(src.file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Field:
			if node.Tag != nil {
				skip[node.Tag] = true
			}
		case *ast.KeyValueExpr:
			if key, ok := node.Key.(*ast.Ident); ok && copylintTriggerKV[key.Name] {
				markSkipped(node.Value)
			}
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiver := ""
			switch x := sel.X.(type) {
			case *ast.Ident:
				receiver = x.Name
			case *ast.SelectorExpr:
				receiver = x.Sel.Name
			}
			if logReceiverNames.MatchString(receiver) {
				markSkipped(node)
				return true
			}
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "fmt" && sel.Sel.Name == "Errorf" && len(node.Args) > 0 {
				if lit, ok := node.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					format, _ := strconv.Unquote(lit.Value)
					if hanPattern.MatchString(format) && wrapVerbPattern.MatchString(format) && !ignored[src.fset.Position(lit.Pos()).Line] {
						counts["errorf_han_wrap"]++
					}
				}
			}
		case *ast.BasicLit:
			if node.Kind != token.STRING || skip[node] || ignored[src.fset.Position(node.Pos()).Line] {
				return true
			}
			if hanPattern.MatchString(node.Value) {
				counts["han_literal"]++
			}
		}
		return true
	})
	return counts
}

// TestCopylintRatchet fails when Go code gains Chinese literals or
// error-wrapping Chinese Errorf calls compared to testdata/copylint.baseline.
// Move the copy into a catalog (docs/i18n.md) instead of raising the
// baseline; after removing literals, lock the progress with
// HARUKI_UPDATE_GOLDEN=1 go test ./internal/i18n/.
func TestCopylintRatchet(t *testing.T) {
	root := repoRoot(t)
	current := map[string]int{}
	for _, src := range parseRepoGo(t, root) {
		if copylintSkipsPath(src.rel) {
			continue
		}
		for rule, n := range copylintCounts(src) {
			current[src.rel+"\t"+rule] = n
		}
	}
	checkRatchet(t, "copylint.baseline", current)
}

func TestCopylintRules(t *testing.T) {
	const code = "package x\n" +
		"\n" +
		"import (\n" +
		"\t\"fmt\"\n" +
		"\t\"log/slog\"\n" +
		")\n" +
		"\n" +
		"type h struct {\n" +
		"\tCommands []string\n" +
		"\tName     string `doc:\"名称\"`\n" +
		"}\n" +
		"\n" +
		"func f(err error) (h, error) {\n" +
		"\tslog.Info(\"日志\")\n" +
		"\t_ = \"stored reason\" + \"原因\" //copylint:ignore stored in the database\n" +
		"\t_ = fmt.Errorf(\"查询失败: %w\", err)\n" +
		"\t_ = fmt.Errorf(\"查询失败\")\n" +
		"\treturn h{Commands: []string{\"/查曲\"}}, fmt.Errorf(\"未找到：%v\", err)\n" +
		"}\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", code, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	counts := copylintCounts(goSource{rel: "internal/x.go", fset: fset, file: file})
	if counts["han_literal"] != 3 || counts["errorf_han_wrap"] != 2 {
		t.Fatalf("copylintCounts() = %v, want 3 literals and 2 wraps", counts)
	}
	for rel, skipped := range map[string]bool{
		"internal/pjsk/handler/music.go":       false,
		"api/bot/pjsk/param_echo.go":           false,
		"main.go":                              false,
		"cmd/importer/main.go":                 true,
		"internal/pjsk/parser/extractor.go":    true,
		"internal/pjsk/filteralias/aliases.go": true,
		"scripts/provision_bot/main.go":        true,
	} {
		if copylintSkipsPath(rel) != skipped {
			t.Errorf("copylintSkipsPath(%q) = %v", rel, !skipped)
		}
	}
}

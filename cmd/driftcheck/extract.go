package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Token kinds. These only affect how drift is grouped in the report; matching is
// always done on the normalized wire value, regardless of kind.
const (
	KindJSON  = "json"  // JSON property name from a struct field tag
	KindEnum  = "enum"  // discriminator / type value (string const or `default:` tag)
	KindModel = "model" // model ID string (e.g. "claude-opus-4-8")
	KindBeta  = "beta"  // anthropic-beta header value
	KindPath  = "path"  // endpoint URL path (normalized, leading "/" and "v1/" stripped)
	KindConst = "const" // any other string-valued const
)

// Token is a single wire-level string the SDK knows about.
type Token struct {
	Value string `json:"value"` // normalized wire value used for matching
	Kind  string `json:"kind"`
	Raw   string `json:"raw,omitempty"` // original literal if it differed from Value
}

// TokenSet is a deduplicated collection of tokens keyed by normalized value.
type TokenSet struct {
	m map[string]Token
}

func newTokenSet() *TokenSet { return &TokenSet{m: map[string]Token{}} }

// add inserts a token. If the value already exists, a more specific kind wins so
// that, e.g., a model ID is not relabelled as a generic const.
func (s *TokenSet) add(value, kind, raw string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if existing, ok := s.m[value]; ok {
		if kindPriority(kind) <= kindPriority(existing.Kind) {
			return
		}
	}
	s.m[value] = Token{Value: value, Kind: kind, Raw: raw}
}

func (s *TokenSet) has(value string) bool {
	_, ok := s.m[value]
	return ok
}

// sorted returns tokens ordered by kind then value for stable reporting.
func (s *TokenSet) sorted() []Token {
	out := make([]Token, 0, len(s.m))
	for _, t := range s.m {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return kindPriority(out[i].Kind) > kindPriority(out[j].Kind)
		}
		return out[i].Value < out[j].Value
	})
	return out
}

func kindPriority(k string) int {
	switch k {
	case KindModel:
		return 5
	case KindBeta:
		return 4
	case KindPath:
		return 3
	case KindEnum:
		return 2
	case KindJSON:
		return 1
	default:
		return 0
	}
}

// extractDir walks root, parses every non-test .go file (skipping the given
// directory names anywhere in the path) and returns the union of tokens found.
func extractDir(root string, skipDirs []string) (*TokenSet, error) {
	set := newTokenSet()
	skip := map[string]bool{}
	for _, d := range skipDirs {
		skip[d] = true
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return extractFile(path, set)
	})
	if err != nil {
		return nil, err
	}
	return set, nil
}

// extractFiles parses a specific list of files and returns their tokens.
func extractFiles(paths []string) (*TokenSet, error) {
	set := newTokenSet()
	for _, p := range paths {
		if err := extractFile(p, set); err != nil {
			return nil, err
		}
	}
	return set, nil
}

func extractFile(path string, set *TokenSet) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.GenDecl:
			if node.Tok == token.CONST {
				for _, spec := range node.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, v := range vs.Values {
						if s, ok := stringLit(v); ok {
							set.add(s, classifyConst(s), "")
						}
					}
				}
			}
		case *ast.StructType:
			for _, field := range node.Fields.List {
				if field.Tag == nil {
					continue
				}
				addTagTokens(field.Tag.Value, set)
			}
		case *ast.BasicLit:
			if node.Kind == token.STRING {
				if s, err := strconv.Unquote(node.Value); err == nil {
					if norm, ok := pathToken(s); ok {
						set.add(norm, KindPath, s)
					}
				}
			}
		}
		return true
	})
	return nil
}

func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// addTagTokens parses a raw Go struct tag literal (including back-quotes) and
// records the json property name and any `default:` discriminator value.
func addTagTokens(rawTag string, set *TokenSet) {
	unq, err := strconv.Unquote(rawTag)
	if err != nil {
		return
	}
	st := reflect.StructTag(unq)
	if jsonTag, ok := st.Lookup("json"); ok {
		name := strings.TrimSpace(strings.Split(jsonTag, ",")[0])
		if name != "" && name != "-" {
			set.add(name, KindJSON, "")
		}
	}
	if def, ok := st.Lookup("default"); ok {
		def = strings.TrimSpace(def)
		if def != "" {
			set.add(def, KindEnum, "")
		}
	}
}

// classifyConst guesses a kind for a string const value for nicer reporting.
func classifyConst(s string) string {
	switch {
	case strings.HasPrefix(s, "claude-"):
		return KindModel
	case isBetaValue(s):
		return KindBeta
	default:
		if norm, ok := pathToken(s); ok {
			_ = norm
			return KindPath
		}
		return KindConst
	}
}

// isBetaValue matches anthropic-beta header strings like "computer-use-2024-10-22".
func isBetaValue(s string) bool {
	// feature-name-YYYY-MM-DD
	if len(s) < 11 {
		return false
	}
	tail := s[len(s)-10:]
	return looksLikeDate(tail) && strings.Contains(s[:len(s)-10], "-")
}

func looksLikeDate(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, r := range s {
		if i == 4 || i == 7 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// pathToken reports whether s looks like an API endpoint path and returns it
// normalized (leading "/" and "v1/" prefixes removed, trailing "/" trimmed) so
// that "/messages" and "v1/messages" compare equal across SDKs.
func pathToken(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	if !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "v1/") {
		return "", false
	}
	if !strings.Contains(s, "/") {
		return "", false
	}
	norm := strings.TrimPrefix(s, "/")
	norm = strings.TrimPrefix(norm, "v1/")
	norm = strings.Trim(norm, "/")
	if norm == "" {
		return "", false
	}
	// Reject things that are clearly not paths (e.g. contain spaces).
	if strings.ContainsAny(norm, " \t") {
		return "", false
	}
	return norm, true
}

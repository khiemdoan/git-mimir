package parser

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// LangID identifies a supported language.
type LangID string

const (
	LangTypeScript LangID = "typescript"
	LangTSX        LangID = "tsx"
	LangJavaScript LangID = "javascript"
	LangGo         LangID = "go"
	LangPython     LangID = "python"
	LangJava       LangID = "java"
	LangRust       LangID = "rust"
	LangCPP        LangID = "cpp"
)

// extToLang maps file extensions to language IDs.
var extToLang = map[string]LangID{
	".ts":  LangTypeScript,
	".tsx": LangTSX,
	".js":  LangJavaScript,
	".jsx": LangJavaScript,
	".go":  LangGo,
	".py":  LangPython,
	".java": LangJava,
	".rs":  LangRust,
	".cpp": LangCPP,
	".cc":  LangCPP,
	".cxx": LangCPP,
}

// LangForExt returns the language ID for a file extension, or "" if unsupported.
func LangForExt(ext string) LangID {
	return extToLang[ext]
}

// grammarForLang returns the tree-sitter grammar for a language.
func grammarForLang(id LangID) *sitter.Language {
	switch id {
	case LangTypeScript:
		return typescript.GetLanguage()
	case LangTSX:
		return tsx.GetLanguage()
	case LangJavaScript:
		return javascript.GetLanguage()
	case LangGo:
		return golang.GetLanguage()
	case LangPython:
		return python.GetLanguage()
	case LangJava:
		return java.GetLanguage()
	case LangRust:
		return rust.GetLanguage()
	case LangCPP:
		return cpp.GetLanguage()
	default:
		return nil
	}
}

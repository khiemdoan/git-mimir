package parser

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
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
	".ts":   LangTypeScript,
	".tsx":  LangTSX,
	".js":   LangJavaScript,
	".jsx":  LangJavaScript,
	".go":   LangGo,
	".py":   LangPython,
	".java": LangJava,
	".rs":   LangRust,
	".cpp":  LangCPP,
	".cc":   LangCPP,
	".cxx":  LangCPP,
}

// LangForExt returns the language ID for a file extension, or "" if unsupported.
func LangForExt(ext string) LangID {
	return extToLang[ext]
}

// grammarForLang returns the tree-sitter grammar for a language.
func grammarForLang(id LangID) *sitter.Language {
	switch id {
	case LangTypeScript:
		return sitter.NewLanguage(typescript.LanguageTypescript())
	case LangTSX:
		return sitter.NewLanguage(typescript.LanguageTSX())
	case LangJavaScript:
		return sitter.NewLanguage(javascript.Language())
	case LangGo:
		return sitter.NewLanguage(golang.Language())
	case LangPython:
		return sitter.NewLanguage(python.Language())
	case LangJava:
		return sitter.NewLanguage(java.Language())
	case LangRust:
		return sitter.NewLanguage(rust.Language())
	case LangCPP:
		return sitter.NewLanguage(cpp.Language())
	default:
		return nil
	}
}

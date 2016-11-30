// Copyright 2014 Jonas mg
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	IMPORT_PATH     = `"github.com/tredoe/gake/tasking"`
	PREFIX_FUNC     = "Task"
	SUFFIX_TASKFILE = "_task.go"
)

// taskPackage represents a package of task files.
type taskPackage struct {
	Name  string
	Files []taskFile
}

// taskFile represents a set of declarations of task functions.
type taskFile struct {
	Name      string
	TaskFuncs []taskFunc
}

// taskFunc represents a task function.
type taskFunc struct {
	Name string
	Doc  string
}

// The "gake" command expects to find task functions in the "*_task.go" files.
//
// A task function is one named TaskXXX (where XXX is any alphanumeric string
// not starting with a lower case letter) and should have the signature,
//
//	func TaskXXX(t *tasking.T) { ... }
func ParseDir(path string) (*taskPackage, error) {
	filter := func(info os.FileInfo) bool {
		if strings.HasSuffix(info.Name(), SUFFIX_TASKFILE) {
			return true
		}
		return false
	}

	fset := token.NewFileSet()

	pkgs, err := parser.ParseDir(fset, path, filter, parser.ParseComments|parser.DeclarationErrors)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, ErrNoTaskfile
	} else if len(pkgs) > 1 {
		return nil, MultiPkgError{path, pkgs}
	}

	pkgName := ""
	for k, _ := range pkgs {
		pkgName = k
		break
	}

	goFiles := make([]taskFile, 0)

	for filename, file := range pkgs[pkgName].Files {
		taskFuncs := make([]taskFunc, 0)

		for _, decl := range file.Decls {
			f, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			funcName := f.Name.Name

			// Check function name
			if !strings.HasPrefix(funcName, PREFIX_FUNC) || len(funcName) <= len(PREFIX_FUNC) {
				continue
			}
			if r, _ := utf8.DecodeRune([]byte(funcName[len(PREFIX_FUNC):])); !unicode.IsUpper(r) && !unicode.IsDigit(r) {
				continue
			}

			// Check function signature

			if f.Type.Results != nil || len(f.Type.Params.List) != 1 {
				return nil, FuncSignError{fset, file, f}
			}
			pointerType, ok := f.Type.Params.List[0].Type.(*ast.StarExpr)
			if !ok {
				return nil, FuncSignError{fset, file, f}
			}
			selector, ok := pointerType.X.(*ast.SelectorExpr)
			if !ok {
				return nil, FuncSignError{fset, file, f}
			}
			if selector.X.(*ast.Ident).Name != "tasking" || selector.Sel.Name != "T" {
				return nil, FuncSignError{fset, file, f}
			}

			taskFuncs = append(taskFuncs, taskFunc{funcName, f.Doc.Text()})
		}
		if len(taskFuncs) == 0 {
			continue
		}

		// Check import path
		hasImportPath := false
		for _, v := range file.Imports {
			if v.Path.Value == IMPORT_PATH {
				hasImportPath = true
				break
			}
		}
		if !hasImportPath {
			return nil, ImportPathError{filename}
		}

		// Check the build constraint
		hasBuildCons := false
		for _, c := range file.Comments {
			comment := c.Text()
			if strings.HasPrefix(comment, "+build") {
				words := strings.Split(comment, " ")
				if words[0] == "+build" && words[1] == "gake\n" {
					// Check whether the build constraint is after of "package"
					if c.Pos() > file.Package {
						return nil, BuildConsPosError{filename}
					}

					hasBuildCons = true
					break
				}
			}
		}
		if !hasBuildCons {
			return nil, BuildConsError{filename}
		}

		goFiles = append(goFiles, taskFile{filename, taskFuncs})
	}

	if len(goFiles) == 0 {
		return nil, ErrNoTask
	}
	return &taskPackage{pkgName, goFiles}, nil
}

// == Errors
//

var (
	ErrNoTask     = errors.New("  [no tasks to run]")
	ErrNoTaskfile = errors.New("  [no task files]")
)

// BuildConsError reports lacking of build constraint.
type BuildConsError struct {
	filename string
}

func (e BuildConsError) Error() string {
	return fmt.Sprintf("%s: no build constraint: \"+build gake\"", e.filename)
}

// BuildConsPosError reports bad position of build constraint.
type BuildConsPosError struct {
	filename string
}

func (e BuildConsPosError) Error() string {
	return fmt.Sprintf("%s: build constraint after of \"package\" directive", e.filename)
}

// FuncSignError represents an incorrect function signature.
type FuncSignError struct {
	fileSet  *token.FileSet
	taskFile *ast.File
	taskFunc *ast.FuncDecl
}

func (e FuncSignError) Error() string {
	return fmt.Sprintf("%s: %s.%s should have the signature func(*tasking.T)",
		e.fileSet.Position(e.taskFile.Pos()),
		e.taskFile.Name.Name,
		e.taskFunc.Name.Name,
	)
}

// ImportPathError represents a file without a necessary import path.
type ImportPathError struct {
	filename string
}

func (e ImportPathError) Error() string {
	return fmt.Sprintf("%s: no import path: %s", e.filename, IMPORT_PATH)
}

// MultiPkgError represents an error due to multiple packages into a same directory.
type MultiPkgError struct {
	path string
	pkgs map[string]*ast.Package
}

func (e MultiPkgError) Error() string {
	msg := make([]string, len(e.pkgs))
	i := 0

	for pkgName, pkg := range e.pkgs {
		files := make([]string, len(pkg.Files))
		j := 0

		for fileName, _ := range pkg.Files {
			files[j] = "'" + fileName + "'"
			j++
		}

		msg[i] = fmt.Sprintf("%q (%s)", pkgName, strings.Join(files, ", "))
		i++
	}

	return fmt.Sprintf("can't load package: found packages %s in '%s'",
		strings.Join(msg, ", "),
		e.path,
	)
}

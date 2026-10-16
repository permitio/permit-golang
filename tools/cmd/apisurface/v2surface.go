package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// v2SurfaceFile is the snapshot of v2's exported API, which TestSurfaceSnapshot keeps in step
// with the source of the permit and models packages. apisurface reads v2's API from it rather
// than from the source, so that it builds without the SDK's packages.
const v2SurfaceFile = "testdata/api-surface.txt"

// v2Surface holds the names of v2's exported declarations, as <package>.<name> and
// <package>.<type>.<name>, and the names of its models types.
type v2Surface struct {
	names  map[string]bool
	models map[string]bool
}

var (
	// packageName matches the package name a line of the snapshot starts with.
	packageName = regexp.MustCompile(`^` + goIdent + `$`)
	// valueDecl matches a constant or variable of the snapshot.
	valueDecl = regexp.MustCompile(`^(?:const|var) (` + goIdent + `)(?: |$)`)
	// funcDecl matches a function of the snapshot.
	funcDecl = regexp.MustCompile(`^func (` + goIdent + `)[\[(]`)
	// methodDecl matches a method of the snapshot, with its receiver type.
	methodDecl = regexp.MustCompile(`^method \(\*?(` + goIdent + `)(?:\[[^()]*\])?\) (` +
		goIdent + `)[\[(]`)
	// typeDecl matches a type of the snapshot, or a member of a struct or interface type, with
	// what follows the type's name and type parameters.
	typeDecl = regexp.MustCompile(`^type (` + goIdent + `)(\[[^()]*?\])?((?: .*)?)$`)
	// leadingName matches the name a member's line starts with, qualified or not.
	leadingName = regexp.MustCompile(`^\*?(?:` + goIdent + `\.)?(` + goIdent + `)`)
)

// readV2Surface reads the names of v2's exported declarations from the snapshot under root.
func readV2Surface(root string) (v2Surface, error) {
	content, err := os.ReadFile(filepath.Join(root, v2SurfaceFile))
	if err != nil {
		return v2Surface{}, fmt.Errorf("reading v2's surface: %w", err)
	}
	s := v2Surface{names: map[string]bool{}, models: map[string]bool{}}
	var problems []error
	err = eachLine(bytes.NewReader(content), func(n int, line string) {
		pkg, decl, _ := strings.Cut(line, ": ")
		name, isType, ok := declName(decl)
		if !ok || !packageName.MatchString(pkg) {
			problems = append(problems, fmt.Errorf("%s:%d: %q is not <package>: <declaration> "+
				"as internal/surface writes it", v2SurfaceFile, n, line))
			return
		}
		if name == "" {
			return
		}
		s.names[pkg+"."+name] = true
		if pkg == "models" && isType {
			s.models[pkg+"."+name] = true
		}
	})
	if err = errors.Join(append(problems, err)...); err != nil {
		return v2Surface{}, fmt.Errorf("reading v2's surface: %w", err)
	}
	return s, nil
}

// declName returns the name of a declaration of the snapshot: Name for a constant, variable,
// type or function, and Type.Name for a method or a member of a struct or interface type. isType
// says that it is a type. It returns "" for an interface's type-set element, such as
// ~int | ~string, which has no name, and false when it does not recognize the declaration.
func declName(decl string) (name string, isType, ok bool) {
	if m := valueDecl.FindStringSubmatch(decl); m != nil {
		return m[1], false, true
	}
	if m := funcDecl.FindStringSubmatch(decl); m != nil {
		return m[1], false, true
	}
	if m := methodDecl.FindStringSubmatch(decl); m != nil {
		return m[1] + "." + m[2], false, true
	}
	m := typeDecl.FindStringSubmatch(decl)
	if m == nil {
		return "", false, false
	}
	typeName, rest := m[1], m[3]
	for _, kind := range []string{" struct, ", " interface, "} {
		if member, isMember := strings.CutPrefix(rest, kind); isMember {
			member = strings.TrimPrefix(member, "embedded ")
			if mm := leadingName.FindStringSubmatch(member); mm != nil {
				return typeName + "." + mm[1], false, true
			}
			return "", false, true
		}
	}
	return typeName, true, true
}

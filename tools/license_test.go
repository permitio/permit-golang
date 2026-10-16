// Package tools holds the tests of the repository's own files that the tools module checks
// directly, such as LICENSE.
package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mitTemplate is the MIT License as https://choosealicense.com/licenses/mit/ gives it, with the
// placeholder copyright line that LICENSE fills in.
const mitTemplate = `MIT License

Copyright (c) [year] [fullname]

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`

// copyrightLine is the third line of LICENSE, in place of the template's placeholder: the years
// and the copyright holder.
const copyrightLine = "Copyright (c) 2023-2026 Permit.io, Ltd."

// TestRepositoryLicense checks that LICENSE is the MIT License word for word, with copyrightLine
// as its third line. pkg.go.dev detects a license by its text and shows a module's documentation
// only when it detects a license it allows, so a changed clause can hide the SDK's documentation.
// It reads LICENSE from the repository root, outside this module, so its name starts with
// TestRepository, which the mutation tests skip.
func TestRepositoryLicense(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(mitTemplate, "\n")
	if want[2] != "Copyright (c) [year] [fullname]" {
		t.Fatalf("line 3 of mitTemplate is %q, want the template's placeholder", want[2])
	}
	want[2] = copyrightLine
	got := strings.Split(string(data), "\n")

	for i := range max(len(got), len(want)) {
		var gotLine, wantLine string
		if i < len(got) {
			gotLine = got[i]
		}
		if i < len(want) {
			wantLine = want[i]
		}
		if gotLine != wantLine {
			t.Fatalf("LICENSE differs from the MIT License at line %d:\n got: %q\nwant: %q\n"+
				"Keep LICENSE the MIT License word for word, with the copyright line %q",
				i+1, gotLine, wantLine, copyrightLine)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("LICENSE has %d lines, want %d", len(got), len(want))
	}
}

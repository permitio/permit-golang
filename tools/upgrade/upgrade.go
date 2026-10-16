// Package upgrade checks that v2 of the SDK reads what v1 wrote. Its test, TestUpgrade, writes
// objects of every kind that v1 writes in an environment with github.com/permitio/permit-golang
// v1.3.0, the v1 release that this module pins, reads each of them with v2, and compares what v2
// read with what v1 sent, field by field, with [Diff]. The module requires v1.3.0 on purpose,
// and v2 and the tools module of this repository through replace directives; it is a test and
// never ships.
//
// With -upgrade.dryrun, TestUpgrade runs offline against the fake of the Permit API in
// tools/e2e/fake, in a scratch environment that it creates and deletes. Without it, it runs
// against the Permit API, in the scratch environment of an end-to-end job whose settings
// .github/scripts/e2e-env.sh create exports, and skips when they are not set. CONTRIBUTING.md
// says how to run it.
package upgrade

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// Decode returns the one JSON value that data holds, such as the body of a request that v1
// sent, with each number as a json.Number, which keeps every digit of its text.
func Decode(data []byte) (any, error) {
	var v any
	if err := wire.Decode(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// View returns model, a model of v2 that holds what v2 read, as encoding/json writes it and
// Decode reads it: a field that the model does not decode is missing from it.
func View(model any) (any, error) {
	data, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("encoding the %T that v2 read: %w", model, err)
	}
	return Decode(data)
}

// Objects returns the n objects that one write of v1 wrote, from bodies, the bodies of the
// requests that v1 sent for it, as Decode returns them: the body itself when it is not a JSON
// array, and else each of its elements, in order. It fails unless v1 sent one body, which holds
// n objects.
func Objects(bodies [][]byte, n int) ([]any, error) {
	if len(bodies) != 1 {
		return nil, fmt.Errorf("v1 sent %d request bodies, want 1", len(bodies))
	}
	written, err := Decode(bodies[0])
	if err != nil {
		return nil, fmt.Errorf("decoding what v1 wrote, %s: %w", bodies[0], err)
	}
	objects := []any{written}
	if items, ok := written.([]any); ok {
		objects = items
	}
	if len(objects) != n {
		return nil, fmt.Errorf("v1 wrote %d objects, %s, want %d", len(objects), bodies[0], n)
	}
	return objects, nil
}

// Without returns written, an object that v1 wrote, as Decode returns it, without its members
// whose names members holds: the members that v1 sends as an instruction that the API does not
// store. It leaves written as it is, and returns it as it is when members is empty. It fails
// when written is not a JSON object, or lacks one of members, as v1 then no longer sends it.
func Without(written any, members ...string) (any, error) {
	if len(members) == 0 {
		return written, nil
	}
	fields, ok := written.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("v1 wrote %s, not an object with the members %q", compact(written),
			members)
	}
	fields = maps.Clone(fields)
	var missing []string
	for _, name := range members {
		if _, ok := fields[name]; !ok {
			missing = append(missing, name)
		}
		delete(fields, name)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("v1 did not write %q, which the comparison would leave out",
			missing)
	}
	return fields, nil
}

// Diff returns each place where read, what v2 read, lacks what written, what v1 wrote, holds,
// or holds another value: the JSON pointer of the place, such as #/attributes/size, and what v1
// wrote and v2 read there. Both are JSON values as Decode returns them.
//
// An object of read must hold each member of the object of written with an equal value, and may
// hold more, such as the ID that the API adds. An array must hold as many elements, equal one by
// one; at a pointer that unordered names, such as #/permissions, it may hold them in another
// order: each element of written must equal an element of its own of read. Two numbers are equal
// when their decimal values are, whatever their text: 1.5e3 equals 1500.0, and 9007199254740993
// does not equal 9007199254740992. Strings, booleans and null must be the same.
func Diff(written, read any, unordered ...string) []string {
	d := differ{unordered: unordered}
	d.diff("#", written, read)
	return d.places
}

// differ collects the places where two JSON values differ.
type differ struct {
	unordered []string
	places    []string
}

// diff adds the places under at, a JSON pointer, where read lacks what written holds or holds
// another value.
func (d *differ) diff(at string, written, read any) {
	switch w := written.(type) {
	case map[string]any:
		r, ok := read.(map[string]any)
		if !ok {
			d.differs(at, written, read)
			return
		}
		for _, name := range slices.Sorted(maps.Keys(w)) {
			member := at + "/" + escape(name)
			value, ok := r[name]
			if !ok {
				d.places = append(d.places, member+": v1 wrote "+compact(w[name])+
					", v2 read nothing")
				continue
			}
			d.diff(member, w[name], value)
		}
	case []any:
		r, ok := read.([]any)
		switch {
		case !ok || len(r) != len(w):
			d.differs(at, written, read)
		case slices.Contains(d.unordered, at):
			if !sameElements(w, r) {
				d.places = append(d.places, at+": v1 wrote "+compact(written)+
					" in any order, v2 read "+compact(read))
			}
		default:
			for i := range w {
				d.diff(at+"/"+strconv.Itoa(i), w[i], r[i])
			}
		}
	case json.Number:
		if r, ok := read.(json.Number); !ok || !sameNumber(w, r) {
			d.differs(at, written, read)
		}
	default:
		if written != read {
			d.differs(at, written, read)
		}
	}
}

// differs adds the place at, where v1 wrote written and v2 read another value, read.
func (d *differ) differs(at string, written, read any) {
	d.places = append(d.places, at+": v1 wrote "+compact(written)+", v2 read "+compact(read))
}

// escape returns name as a token of a JSON pointer, with ~ written ~0 and / written ~1.
func escape(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
}

// sameNumber reports whether a and b have the same decimal value.
func sameNumber(a, b json.Number) bool {
	x, okA := new(big.Rat).SetString(a.String())
	y, okB := new(big.Rat).SetString(b.String())
	return okA && okB && x.Cmp(y) == 0
}

// sameElements reports whether each element of written equals an element of its own of read, as
// Diff compares them, in any order. It pairs them as a bipartite matching does, so an element of
// written that more than one element of read equals does not take the one that another needs.
func sameElements(written, read []any) bool {
	// owner[j] is the index of the element of written that read[j] is paired with, or -1.
	owner := make([]int, len(read))
	for j := range owner {
		owner[j] = -1
	}
	var pair func(i int, tried []bool) bool
	pair = func(i int, tried []bool) bool {
		for j := range read {
			if tried[j] || len(Diff(written[i], read[j])) > 0 {
				continue
			}
			tried[j] = true
			if owner[j] < 0 || pair(owner[j], tried) {
				owner[j] = i
				return true
			}
		}
		return false
	}
	for i := range written {
		if !pair(i, make([]bool, len(read))) {
			return false
		}
	}
	return true
}

// compact returns v as JSON text on one line, with the members of each object sorted by name and
// with <, > and & as they are, for a message.
func compact(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprintf("%#v", v)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

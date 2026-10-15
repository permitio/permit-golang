// Package drift holds the drift tests of the SDK against the spec snapshots in tools/specs.
// model-map.yaml maps each type of package models, and each decision type of the root package
// that the PDP's requests and replies hold, to its schema, and to the schemas of other specs that
// describe it too, with a required flag and a null policy for each field of a struct; the tests
// compare each type with each of its schemas in both directions, round-trip the maximal values of
// each schema through the type with a strict decoder, compare the RequestModel and ResponseModel
// of each operation of the registry with its request and reply schemas, and validate each reply
// fixture in the root's testdata/replies against its reply schema, in the spec that lists the
// operation and in each other spec that lists its route. allow.yaml accepts the findings that
// come from known defects of the specs, each with a reason and a trigger, and an entry that
// accepts nothing fails. The tests of waits_test.go check the routes on which the PDP container
// waits for sync, which the spec of the container lists, against the root's doc.go, the registry
// and coverage.yaml.
//
// The checks share one walker of the part of JSON Schema that the snapshots use. A schema with
// a keyword that the walker does not know stops the check that meets it, which then reports DID
// NOT RUN with the keyword's name, so that nothing is skipped unseen. The walker is the code of
// the package outside its tests, so that the end-to-end dry run checks each request body that
// its fake of the Permit API receives with it too: [ReadSpecs] reads the snapshots, and
// [Specs.CheckRequestBody] checks a body against the request schema of its operation.
//
// The tests whose names start with TestRepository read the files of the repository: package
// models, the root package, the root's testdata/replies, and the snapshots in tools/specs, or
// those in the directory that -specs names, such as the live snapshots of specsnap -check-live
// -out. The other tests check the walker and the checks on the fixtures in testdata.
// CONTRIBUTING.md, section Model drift, says how to write a row of the map, an entry of the
// allowlist and a reply fixture.
package drift

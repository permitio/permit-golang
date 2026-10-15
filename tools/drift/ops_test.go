package drift

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/permitio/permit-golang/v2/internal/rest"
)

// operation returns the findings of op: its RequestModel against the request body of its
// operation in the specs, and its ResponseModel against each of its replies in 200-299 that has
// a JSON body, both in the spec that lists its ID and in each other spec that lists its method
// and path, as the cloud PDP lists the routes of the PDP container that it serves. The request
// holds the model when one of its alternatives does, as the SDK chooses what it sends; a reply
// holds it only when each of its alternatives does, as the API chooses. An alternative holds the
// model when it is the model's schema; for a RequestModel or a ResponseModel of [] and a name, a
// body that is a JSON array, such as the request of a bulk route, when it is an array of the
// model instead; and in the reply of a list, an operation with a MaxPerPage, when it is an array
// or a paginated envelope of it instead. Its error is an *unsupportedError when a schema holds
// what the walker does not implement.
func (c *comparer) operation(op rest.Op) ([]finding, error) {
	var missing []finding
	request, _ := requestModel(op)
	response, _ := responseModel(op)
	for _, name := range []string{request, response} {
		if _, ok := c.rows[name]; name != "" && !ok {
			missing = append(missing, finding{text: "the operation " + op.ID + ": its model " +
				name + " has no row under models in model-map.yaml"})
		}
	}
	if missing != nil {
		return missing, nil
	}
	o, ok := findOperation(c.specs, op.ID)
	if !ok {
		if op.RequestModel == "" && op.ResponseModel == "" {
			return nil, nil
		}
		return []finding{{"operation " + op.ID, "no spec lists the operation, so its models " +
			"cannot be checked"}}, nil
	}
	var found []finding
	for _, each := range append([]specOperation{o}, sameRoute(c.specs, o)...) {
		more, err := c.request(op, each)
		if err != nil {
			return nil, err
		}
		found = append(found, more...)
		if more, err = c.response(op, each); err != nil {
			return nil, err
		}
		found = append(found, more...)
	}
	return found, nil
}

// label returns the name of op in the keys of the findings against o: its ID, and @ and the spec
// of o when o is the operation of another spec with the same route, such as
// is_allowed_allowed_post@pdp-cloud.
func label(op rest.Op, o specOperation) string {
	if o.node["operationId"] == op.ID {
		return op.ID
	}
	return op.ID + "@" + o.spec.name
}

// request returns the findings of the RequestModel of op against the request body of o.
func (c *comparer) request(op rest.Op, o specOperation) ([]finding, error) {
	key := "operation " + label(op, o) + " request"
	at := o.at + "/requestBody"
	body, _ := o.node["requestBody"].(map[string]any)
	if ref, ok := body["$ref"].(string); ok {
		target, _, err := o.spec.resolve(ref)
		if err != nil {
			return nil, err
		}
		body, _ = target.(map[string]any)
		at = ref
	}
	schema, at, hasJSON := jsonBody(body, at)
	switch {
	case op.RequestModel == "" && body["required"] == true:
		return []finding{{key, "the spec requires a request body, and the operation has no " +
			"RequestModel"}}, nil
	case op.RequestModel == "":
		return nil, nil
	case !hasJSON:
		return []finding{{key, "its RequestModel is " + op.RequestModel + ", and the spec gives " +
			"the operation no JSON request body"}}, nil
	}
	model, array := requestModel(op)
	check := bodyCheck{model: model, key: key, array: array, field: "RequestModel"}
	return c.body(check, schema, at, o.spec)
}

// response returns the findings of the ResponseModel of op against each reply of o in 200-299
// with a JSON body.
func (c *comparer) response(op rest.Op, o specOperation) ([]finding, error) {
	if op.ResponseModel == "" {
		return nil, nil
	}
	key := "operation " + label(op, o) + " response"
	model, array := responseModel(op)
	check := bodyCheck{
		model: model, key: key, every: true, list: op.MaxPerPage > 0, array: array,
		field: "ResponseModel",
	}
	responses, _ := o.node["responses"].(map[string]any)
	var found []finding
	replies := 0
	for _, status := range slices.Sorted(maps.Keys(responses)) {
		code, err := strconv.Atoi(status)
		reply, _ := responses[status].(map[string]any)
		schema, at, hasJSON := jsonBody(reply, o.at+"/responses/"+status)
		if err != nil || code < 200 || code > 299 || !hasJSON {
			continue
		}
		replies++
		more, err := c.body(check, schema, at, o.spec)
		if err != nil {
			return nil, err
		}
		for _, f := range more {
			found = append(found, finding{f.key, "its " + status + " reply: " + f.text})
		}
	}
	if replies == 0 {
		found = append(found, finding{key, "its ResponseModel is " + op.ResponseModel + ", and " +
			"no reply of the operation in 200-299 has a JSON body in the spec"})
	}
	return found, nil
}

// bodyCheck is what body checks a body for.
type bodyCheck struct {
	// model is the name of the model that the body holds, which has a row.
	model string

	// key is the key of the findings.
	key string

	// every says that each alternative of the body holds the model, rather than one of them.
	every bool

	// list says that the body is a list of the model, rather than the model itself.
	list bool

	// array says that the body is a JSON array of the model, rather than the model itself, as
	// the request of a bulk route is.
	array bool

	// field is the field of the operation that names the model, RequestModel or ResponseModel,
	// which the finding of an array names.
	field string
}

// body returns the findings of check against schema, a body that at locates in s: the model's row
// maps it to a schema of s that the body holds, in each of its alternatives with check.every, and
// in one of them otherwise. An alternative holds the model when it is the model's schema; with
// check.array, when it is an array whose items are instead; and with check.list, when it is an
// array or a paginated envelope whose items are instead.
func (c *comparer) body(check bodyCheck, schema any, at string, s *spec) ([]finding, error) {
	row := c.rows[check.model]
	mapped, inSpec := row.schemaIn(s.name)
	if err := s.audit(schema, at); err != nil {
		return nil, err
	}
	held, err := s.heldModels(schema, at)
	if err != nil {
		return nil, err
	}
	holds := func(h heldBody) bool {
		switch {
		case check.list:
			return slices.Contains(h.items, mapped)
		case check.array:
			return h.list == "an array" && slices.Contains(h.items, mapped)
		default:
			return h.name == mapped
		}
	}
	fits := slices.ContainsFunc(held, holds)
	if check.every {
		fits = !slices.ContainsFunc(held, func(h heldBody) bool { return !holds(h) })
	}
	if inSpec && fits {
		return nil, nil
	}
	described := make([]string, 0, len(held))
	for _, h := range held {
		described = append(described, h.String())
	}
	text := "the model " + check.model + " maps to " + row.describe() + ", and the body " +
		"holds " + strings.Join(described, ", or ") + " of " + s.name
	switch {
	case check.list:
		text += "; the reply of a list is an array or a paginated envelope of its model"
	case check.array:
		text += "; the " + check.field + " []" + check.model + " is a JSON array of the model"
	}
	return []finding{{check.key, text}}, nil
}

// heldBody is what an alternative of a body holds.
type heldBody struct {
	// name is the name of the alternative's schema of components.schemas; empty for an inline
	// schema.
	name string

	// list says how the alternative lists items, "an array" or "a paginated envelope"; empty
	// when it is not a list.
	list string

	// items are the names of the schemas of components.schemas that the list's items are.
	items []string
}

// String returns h as a finding names it, such as EnvRead, an array of EnvRead, or a paginated
// envelope PaginatedResult_EnvRead_ of EnvRead.
func (h heldBody) String() string {
	if h.list == "" {
		if h.name == "" {
			return "an inline schema"
		}
		return h.name
	}
	items := "an inline schema"
	if len(h.items) > 0 {
		items = strings.Join(h.items, " or ")
	}
	if h.name == "" {
		return h.list + " of " + items
	}
	return h.list + " " + h.name + " of " + items
}

// heldModels returns what each alternative of schema, which at locates, holds: its own schema of
// components.schemas, and the schemas of the items of an array, or of a paginated envelope, a
// schema of components.schemas whose data is an array and whose other properties are counts.
func (s *spec) heldModels(schema any, at string) ([]heldBody, error) {
	alts, err := s.alternatives(schema, at)
	if err != nil {
		return nil, err
	}
	held := make([]heldBody, 0, len(alts))
	for _, alt := range alts {
		h := heldBody{name: alt.name}
		var items any
		switch {
		case typeOf(alt.schema) == "array":
			h.list, items = "an array", alt.schema["items"]
		case alt.name != "" && envelopeItems(alt.schema) != nil:
			h.list, items = "a paginated envelope", envelopeItems(alt.schema)
		}
		if items != nil {
			itemAlts, err := s.alternatives(items, at)
			if err != nil {
				return nil, err
			}
			for _, item := range itemAlts {
				if item.name != "" {
					h.items = append(h.items, item.name)
				}
			}
		}
		held = append(held, h)
	}
	return held, nil
}

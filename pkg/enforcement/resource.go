package enforcement

type Resource struct {
	Type       string            `json:"type,omitempty"`
	ID         string            `json:"id,omitempty"`
	Tenant     string            `json:"tenant,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Context    map[string]string `json:"context,omitempty"`
}

func ResourceBuilder(resourceType string) *Resource {
	return &Resource{
		Type: resourceType,
	}
}

func (r *Resource) WithID(ID string) *Resource {
	r.ID = ID
	return r
}

func (r *Resource) WithTenant(tenant string) *Resource {
	r.Tenant = tenant
	return r
}

func (r *Resource) WithAttributes(attributes map[string]string) *Resource {
	r.Attributes = attributes
	return r
}

func (r *Resource) WithContext(context map[string]string) *Resource {
	r.Context = context
	return r
}

func (r *Resource) Build() Resource {
	return *r
}

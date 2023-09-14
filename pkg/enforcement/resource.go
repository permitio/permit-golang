package enforcement

type ResourceI interface {
	GetID() string
	GetType() string
	GetTenant() string
	GetAttributes() map[string]string
	GetContext() map[string]string
}

type Resource struct {
	Type       string            `json:"type,omitempty"`
	ID         string            `json:"id,omitempty"`
	Tenant     string            `json:"tenant,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Context    map[string]string `json:"context,omitempty"`
}

func (r *Resource) GetID() string {
	return r.ID
}

func (r *Resource) GetType() string {
	return r.Type
}

func (r *Resource) GetTenant() string {
	return r.Tenant
}

func (r *Resource) GetAttributes() map[string]string {
	return r.Attributes
}

func (r *Resource) GetContext() map[string]string {
	return r.Context
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
	if r.Tenant == "" {
		r.Tenant = DefaultTenant
	}
	return *r
}

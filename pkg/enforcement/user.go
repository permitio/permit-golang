package enforcement

type AssignedRole struct {
	Role   string
	Tenant string
}

type User struct {
	Key        string                 `json:"key,omitempty"`
	FirstName  string                 `json:"first_name,omitempty"`
	LastName   string                 `json:"last_name,omitempty"`
	Email      string                 `json:"email,omitempty"`
	Roles      []AssignedRole         `json:"roles,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

func UserBuilder(key string) *User {
	return &User{
		Key: key,
	}
}

func (u *User) WithFirstName(firstName string) *User {
	u.FirstName = firstName
	return u
}

func (u *User) WithLastName(lastName string) *User {
	u.LastName = lastName
	return u
}

func (u *User) WithEmail(email string) *User {
	u.Email = email
	return u
}

func (u *User) WithRoles(roles []AssignedRole) *User {
	u.Roles = roles
	return u
}

func (u *User) WithAttributes(attributes map[string]interface{}) *User {
	u.Attributes = attributes
	return u
}

func (u *User) Build() User {
	return *u
}

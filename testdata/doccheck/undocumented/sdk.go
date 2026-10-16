package sdk

import "fmt"

const Version = "1.0.0"

// Grouped values have the comment of their group.
const (
	A = 1
	B = 2
)

const (
	// C has a comment of its own.
	C = 3
	D = 4 // D has a line comment.
	E = 5
)

var Debug bool

func Helper() {}

// Base is embedded in Client.
type Base struct{}

func (Base) Hello() {}

func (*Base) Bye() {}

type Client struct {
	Users *UsersService
	// HTTP has a comment.
	HTTP    string
	Timeout int // Timeout bounds each call.
	Base
	hidden
	apiKey string
}

func New() *Client { return &Client{} }

type hidden struct{}

func (hidden) Get() {}

// Kind is a kind of user.
type Kind string

const KindUser Kind = "user"

// Getter gets.
type Getter interface {
	Get() string
	fmt.Stringer
	// Put puts.
	Put(string)
	unexported()
	// getter adds get.
	getter
}

// getter gets.
type getter interface {
	get()
	// Value returns the value.
	Value() string
}

// Reader gets Get and Put from Getter, Value from getter through Getter, and Read from reader. It
// declares Put itself.
type Reader struct {
	// Getter gets.
	Getter
	reader
}

// Put puts the value.
func (Reader) Put(string) {}

// reader reads.
type reader interface {
	Read() string
}

// Wrapper gets the methods of Reader.
type Wrapper struct {
	// Reader reads.
	Reader
}

// Aliased gets Read from reader through an alias.
type Aliased struct {
	readAlias
}

type readAlias = reader

// UsersService reads and writes users.
type UsersService struct{}

func (s *UsersService) List() {}

// Node embeds Tree, which embeds Node.
type Node struct {
	// Tree holds the node.
	*Tree
}

// Tree embeds Node, which embeds Tree.
type Tree struct {
	// Node is the root of the tree.
	*Node
}

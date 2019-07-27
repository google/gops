package treeprint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOneNode(t *testing.T) {
	assert := assert.New(t)

	tree := New()
	tree.AddNode("hello")
	actual := tree.String()
	expected := `.
└── hello
`
	assert.Equal(expected, actual)
}

func TestMetaNode(t *testing.T) {
	assert := assert.New(t)

	tree := New()
	tree.AddMetaNode(123, "hello")
	tree.AddMetaNode([]struct{}{}, "world")
	actual := tree.String()
	expected := `.
├── [123]  hello
└── [[]]  world
`
	assert.Equal(expected, actual)
}

func TestTwoNodes(t *testing.T) {
	assert := assert.New(t)

	tree := New()
	tree.AddNode("hello")
	tree.AddNode("world")
	actual := tree.String()
	expected := `.
├── hello
└── world
`
	assert.Equal(expected, actual)
}

func TestLevel(t *testing.T) {
	assert := assert.New(t)

	tree := New()
	tree.AddBranch("hello").AddNode("my friend").AddNode("lol")
	tree.AddNode("world")
	actual := tree.String()
	expected := `.
├── hello
│   ├── my friend
│   └── lol
└── world
`
	assert.Equal(expected, actual)
}

func TestNamedRoot(t *testing.T) {
	assert := assert.New(t)

	tree := New()
	tree.AddBranch("hello").AddNode("my friend").AddNode("lol")
	tree.AddNode("world")
	tree.SetValue("friends")
	actual := tree.String()
	expected := `friends
├── hello
│   ├── my friend
│   └── lol
└── world
`
	assert.Equal(expected, actual)
}

func TestDeepLevel(t *testing.T) {
	assert := assert.New(t)

	tree := New()
	one := tree.AddBranch("one")
	one.AddNode("subnode1").AddNode("subnode2")
	one.AddBranch("two").
		AddNode("subnode1").AddNode("subnode2").
		AddBranch("three").
		AddNode("subnode1").AddNode("subnode2")
	one.AddNode("subnode3")
	tree.AddNode("outernode")

	actual := tree.String()
	expected := `.
├── one
│   ├── subnode1
│   ├── subnode2
│   ├── two
│   │   ├── subnode1
│   │   ├── subnode2
│   │   └── three
│   │       ├── subnode1
│   │       └── subnode2
│   └── subnode3
└── outernode
`
	assert.Equal(expected, actual)
}

func TestComplex(t *testing.T) {
	assert := assert.New(t)

	tree := New()
	tree.AddNode("Dockerfile")
	tree.AddNode("Makefile")
	tree.AddNode("aws.sh")
	tree.AddMetaBranch(" 204", "bin").
		AddNode("dbmaker").AddNode("someserver").AddNode("testtool")
	tree.AddMetaBranch(" 374", "deploy").
		AddNode("Makefile").AddNode("bootstrap.sh")
	tree.AddMetaNode("122K", "testtool.a")

	actual := tree.String()
	expected := `.
├── Dockerfile
├── Makefile
├── aws.sh
├── [ 204]  bin
│   ├── dbmaker
│   ├── someserver
│   └── testtool
├── [ 374]  deploy
│   ├── Makefile
│   └── bootstrap.sh
└── [122K]  testtool.a
`
	assert.Equal(expected, actual)
}

func TestIndirectOrder(t *testing.T) {
	assert := assert.New(t)

	tree := New()
	tree.AddBranch("one").AddNode("two")
	foo := tree.AddBranch("foo")
	foo.AddBranch("bar").AddNode("a").AddNode("b").AddNode("c")
	foo.AddNode("end")

	actual := tree.String()
	expected := `.
├── one
│   └── two
└── foo
    ├── bar
    │   ├── a
    │   ├── b
    │   └── c
    └── end
`
	assert.Equal(expected, actual)
}

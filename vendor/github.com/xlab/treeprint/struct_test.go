package treeprint

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type nameStruct struct {
	One   string `json:"one" tree:"one"`
	Two   int    `tree:"two"`
	Three struct {
		SubOne   []string
		SubTwo   []interface{}
		SubThree struct {
			InnerOne   *float64  `tree:"inner_one,omitempty"`
			InnerTwo   *struct{} `tree:",omitempty"`
			InnerThree *float64  `tree:"inner_three"`
		}
	}
}

func TestFromStructName(t *testing.T) {
	assert := assert.New(t)

	tree, err := FromStruct(nameStruct{}, StructNameTree)
	assert.NoError(err)

	actual := tree.String()
	expected := `.
├── one
├── two
└── Three
    ├── SubOne
    ├── SubTwo
    └── SubThree
        └── inner_three
`
	assert.Equal(expected, actual)
}

func TestFromStructTags(t *testing.T) {
	assert := assert.New(t)

	tree, err := FromStruct(nameStruct{}, StructTagTree)
	assert.NoError(err)

	actual := tree.String()
	expected := `.
├── [json:"one"]  one
├── []  two
└── []  Three
    ├── []  SubOne
    ├── []  SubTwo
    └── []  SubThree
        └── []  inner_three
`
	assert.Equal(expected, actual)
}

type typeStruct struct {
	One   string `json:"one" tree:"one"`
	Two   int    `tree:"two"`
	Three subtypeStruct
}

type subtypeStruct struct {
	SubOne   []string
	SubTwo   []interface{}
	SubThree subsubTypeStruct
}

type subsubTypeStruct struct {
	InnerOne   *float64  `tree:"inner_one,omitempty"`
	InnerTwo   *struct{} `tree:",omitempty"`
	InnerThree *float64  `tree:"inner_three"`
}

func TestFromStructType(t *testing.T) {
	assert := assert.New(t)

	tree, err := FromStruct(typeStruct{}, StructTypeTree)
	assert.NoError(err)

	actual := tree.String()
	expected := `.
├── [string]  one
├── [int]  two
└── [treeprint.subtypeStruct]  Three
    ├── [[]string]  SubOne
    ├── [[]interface {}]  SubTwo
    └── [treeprint.subsubTypeStruct]  SubThree
        └── [*float64]  inner_three
`
	assert.Equal(expected, actual)
}

func TestFromStructTypeSize(t *testing.T) {
	assert := assert.New(t)

	tree, err := FromStruct(typeStruct{}, StructTypeSizeTree)
	assert.NoError(err)

	actual := tree.String()
	expected := `.
├── [16]  one
├── [8]  two
└── [72]  Three
    ├── [24]  SubOne
    ├── [24]  SubTwo
    └── [24]  SubThree
        └── [8]  inner_three
`
	assert.Equal(expected, actual)
}

type valueStruct struct {
	Name string
	Bio  struct {
		Age  int
		City string
		Meta interface{}
	}
}

func TestFromStructValue(t *testing.T) {
	assert := assert.New(t)

	val := valueStruct{
		Name: "Max",
	}
	val.Bio.Age = 100
	val.Bio.City = "NYC"
	val.Bio.Meta = []byte("hello")
	tree, err := FromStruct(val, StructValueTree)
	assert.NoError(err)

	actual := tree.String()
	expected := `.
├── [Max]  Name
└── Bio
    ├── [100]  Age
    ├── [NYC]  City
    └── [[104 101 108 108 111]]  Meta
`
	assert.Equal(expected, actual)
}

func TestFromStructWithMeta(t *testing.T) {
	assert := assert.New(t)

	val := valueStruct{
		Name: "Max",
	}
	val.Bio.Age = 100
	val.Bio.City = "NYC"
	val.Bio.Meta = []byte("hello")
	tree, err := FromStructWithMeta(val, func(_ string, v interface{}) (string, bool) {
		return fmt.Sprintf("lol %T", v), true
	})
	assert.NoError(err)

	actual := tree.String()
	expected := `.
├── [lol string]  Name
└── [lol struct { Age int; City string; Meta interface {} }]  Bio
    ├── [lol int]  Age
    ├── [lol string]  City
    └── [lol []uint8]  Meta
`
	assert.Equal(expected, actual)
}

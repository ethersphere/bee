// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Note: the following code is derived (borrows) from: github.com/go-logr/logr

package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// substr is handled via reflection instead of type assertions.
type substr string

// point implements encoding.TextMarshaller and can be used as a map key.
type point struct{ x, y int }

func (p point) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("(%d, %d)", p.x, p.y)), nil
}

// pointErr implements encoding.TextMarshaler but returns an error.
type pointErr struct{ x, y int }

func (p pointErr) MarshalText() ([]byte, error) {
	return nil, fmt.Errorf("uh oh: %d, %d", p.x, p.y)
}

// nolint:errname
// marshalerTest expect to result in the MarshalLog() value when logged.
type marshalerTest struct{ val string }

func (marshalerTest) MarshalLog() interface{} {
	return struct{ Inner string }{"I am a log.Marshaler"}
}
func (marshalerTest) String() string {
	return "String(): you should not see this"
}
func (marshalerTest) Error() string {
	return "Error(): you should not see this"
}

// nolint:errname
// marshalerPanicTest expect this to result in a panic when logged.
type marshalerPanicTest struct{ val string }

func (marshalerPanicTest) MarshalLog() interface{} {
	panic("marshalerPanicTest")
}

// nolint:errname
// stringerTest expect this to result in the String() value when logged.
type stringerTest struct{ val string }

func (stringerTest) String() string {
	return "I am a fmt.Stringer"
}
func (stringerTest) Error() string {
	return "Error(): you should not see this"
}

// stringerPanicTest expect this to result in a panic when logged.
type stringerPanicTest struct{ val string }

func (stringerPanicTest) String() string {
	panic("stringerPanicTest")
}

// nolint:errname
// errorTest expect this to result in the Error() value when logged.
type errorTest struct{ val string }

func (errorTest) Error() string {
	return "I am an error"
}

// nolint:errname
// errorPanicTest expect this to result in a panic when logged.
type errorPanicTest struct{ val string }

func (errorPanicTest) Error() string {
	panic("errorPanicTest")
}

type (
	jsonTagsStringTest struct {
		String1 string `json:"string1"`           // renamed
		String2 string `json:"-"`                 // ignored
		String3 string `json:"-,"`                // named "-"
		String4 string `json:"string4,omitempty"` // renamed, ignore if empty
		String5 string `json:","`                 // no-op
		String6 string `json:",omitempty"`        // ignore if empty
	}

	jsonTagsBoolTest struct {
		Bool1 bool `json:"bool1"`           // renamed
		Bool2 bool `json:"-"`               // ignored
		Bool3 bool `json:"-,"`              // named "-"
		Bool4 bool `json:"bool4,omitempty"` // renamed, ignore if empty
		Bool5 bool `json:","`               // no-op
		Bool6 bool `json:",omitempty"`      // ignore if empty
	}

	jsonTagsIntTest struct {
		Int1 int `json:"int1"`           // renamed
		Int2 int `json:"-"`              // ignored
		Int3 int `json:"-,"`             // named "-"
		Int4 int `json:"int4,omitempty"` // renamed, ignore if empty
		Int5 int `json:","`              // no-op
		Int6 int `json:",omitempty"`     // ignore if empty
	}

	jsonTagsUintTest struct {
		Uint1 uint `json:"uint1"`           // renamed
		Uint2 uint `json:"-"`               // ignored
		Uint3 uint `json:"-,"`              // named "-"
		Uint4 uint `json:"uint4,omitempty"` // renamed, ignore if empty
		Uint5 uint `json:","`               // no-op
		Uint6 uint `json:",omitempty"`      // ignore if empty
	}

	jsonTagsFloatTest struct {
		Float1 float64 `json:"float1"`           // renamed
		Float2 float64 `json:"-"`                // ignored
		Float3 float64 `json:"-,"`               // named "-"
		Float4 float64 `json:"float4,omitempty"` // renamed, ignore if empty
		Float5 float64 `json:","`                // no-op
		Float6 float64 `json:",omitempty"`       // ignore if empty
	}

	jsonTagsComplexTest struct {
		Complex1 complex128 `json:"complex1"`           // renamed
		Complex2 complex128 `json:"-"`                  // ignored
		Complex3 complex128 `json:"-,"`                 // named "-"
		Complex4 complex128 `json:"complex4,omitempty"` // renamed, ignore if empty
		Complex5 complex128 `json:","`                  // no-op
		Complex6 complex128 `json:",omitempty"`         // ignore if empty
	}

	jsonTagsPtrTest struct {
		Ptr1 *string `json:"ptr1"`           // renamed
		Ptr2 *string `json:"-"`              // ignored
		Ptr3 *string `json:"-,"`             // named "-"
		Ptr4 *string `json:"ptr4,omitempty"` // renamed, ignore if empty
		Ptr5 *string `json:","`              // no-op
		Ptr6 *string `json:",omitempty"`     // ignore if empty
	}

	jsonTagsArrayTest struct {
		Array1 [2]string `json:"array1"`           // renamed
		Array2 [2]string `json:"-"`                // ignored
		Array3 [2]string `json:"-,"`               // named "-"
		Array4 [2]string `json:"array4,omitempty"` // renamed, ignore if empty
		Array5 [2]string `json:","`                // no-op
		Array6 [2]string `json:",omitempty"`       // ignore if empty
	}

	jsonTagsSliceTest struct {
		Slice1 []string `json:"slice1"`           // renamed
		Slice2 []string `json:"-"`                // ignored
		Slice3 []string `json:"-,"`               // named "-"
		Slice4 []string `json:"slice4,omitempty"` // renamed, ignore if empty
		Slice5 []string `json:","`                // no-op
		Slice6 []string `json:",omitempty"`       // ignore if empty
	}

	jsonTagsMapTest struct {
		Map1 map[string]string `json:"map1"`           // renamed
		Map2 map[string]string `json:"-"`              // ignored
		Map3 map[string]string `json:"-,"`             // named "-"
		Map4 map[string]string `json:"map4,omitempty"` // renamed, ignore if empty
		Map5 map[string]string `json:","`              // no-op
		Map6 map[string]string `json:",omitempty"`     // ignore if empty
	}

	InnerStructTest struct{ Inner string }
	InnerIntTest    int
	InnerMapTest    map[string]string
	InnerSliceTest  []string

	embedStructTest struct {
		InnerStructTest
		Outer string
	}

	embedNonStructTest struct {
		InnerIntTest
		InnerMapTest
		InnerSliceTest
	}

	Inner1Test InnerStructTest
	Inner2Test InnerStructTest
	Inner3Test InnerStructTest
	Inner4Test InnerStructTest
	Inner5Test InnerStructTest
	Inner6Test InnerStructTest

	embedJSONTagsTest struct {
		Outer      string
		Inner1Test `json:"inner1"`
		Inner2Test `json:"-"`
		Inner3Test `json:"-,"`
		Inner4Test `json:"inner4,omitempty"`
		Inner5Test `json:","`
		Inner6Test `json:"inner6,omitempty"`
	}
)

func TestPretty(t *testing.T) {
	intPtr := func(i int) *int { return &i }
	strPtr := func(s string) *string { return &s }

	testCases := []struct {
		val interface{}
		exp string // used in testCases where JSON can't handle it
	}{{
		val: "strval",
	}, {
		val: "strval\nwith\t\"escapes\"",
	}, {
		val: substr("substrval"),
	}, {
		val: substr("substrval\nwith\t\"escapes\""),
	}, {
		val: true,
	}, {
		val: false,
	}, {
		val: 93,
	}, {
		val: int8(93),
	}, {
		val: int16(93),
	}, {
		val: int32(93),
	}, {
		val: int64(93),
	}, {
		val: -93,
	}, {
		val: int8(-93),
	}, {
		val: int16(-93),
	}, {
		val: int32(-93),
	}, {
		val: int64(-93),
	}, {
		val: uint(93),
	}, {
		val: uint8(93),
	}, {
		val: uint16(93),
	}, {
		val: uint32(93),
	}, {
		val: uint64(93),
	}, {
		val: uintptr(93),
	}, {
		val: float32(93.76),
	}, {
		val: 93.76,
	}, {
		val: complex64(93i),
		exp: `"(0+93i)"`,
	}, {
		val: 93i,
		exp: `"(0+93i)"`,
	}, {
		val: intPtr(93),
	}, {
		val: strPtr("pstrval"),
	}, {
		val: []int{},
	}, {
		val: []int(nil),
		exp: `[]`,
	}, {
		val: []int{9, 3, 7, 6},
	}, {
		val: []string{"str", "with\tescape"},
	}, {
		val: []substr{"substr", "with\tescape"},
	}, {
		val: [4]int{9, 3, 7, 6},
	}, {
		val: [2]string{"str", "with\tescape"},
	}, {
		val: [2]substr{"substr", "with\tescape"},
	}, {
		val: struct {
			Int         int
			notExported string
			String      string
		}{
			93, "you should not see this", "seventy-six",
		},
	}, {
		val: map[string]int{},
	}, {
		val: map[string]int(nil),
		exp: `{}`,
	}, {
		val: map[string]int{
			"nine": 3,
		},
	}, {
		val: map[string]int{
			"with\tescape": 76,
		},
	}, {
		val: map[substr]int{
			"nine": 3,
		},
	}, {
		val: map[substr]int{
			"with\tescape": 76,
		},
	}, {
		val: map[int]int{
			9: 3,
		},
	}, {
		val: map[float64]int{
			9.5: 3,
		},
		exp: `{"9.5":3}`,
	}, {
		val: map[point]int{
			{x: 1, y: 2}: 3,
		},
	}, {
		val: map[pointErr]int{
			{x: 1, y: 2}: 3,
		},
		exp: `{"<error-MarshalText: uh oh: 1, 2>":3}`,
	}, {
		val: struct {
			X int `json:"x"`
			Y int `json:"y"`
		}{
			93, 76,
		},
	}, {
		val: struct {
			X []int
			Y map[int]int
			Z struct{ P, Q int }
		}{
			[]int{9, 3, 7, 6},
			map[int]int{9: 3},
			struct{ P, Q int }{9, 3},
		},
	}, {
		val: []struct{ X, Y string }{
			{"nine", "three"},
			{"seven", "six"},
			{"with\t", "\tescapes"},
		},
	}, {
		val: struct {
			A *int
			B *int
			C interface{}
			D interface{}
		}{
			B: intPtr(1),
			D: interface{}(2),
		},
	}, {
		val: marshalerTest{"foobar"},
		exp: `{"Inner":"I am a log.Marshaler"}`,
	}, {
		val: &marshalerTest{"foobar"},
		exp: `{"Inner":"I am a log.Marshaler"}`,
	}, {
		val: (*marshalerTest)(nil),
		exp: `"<panic: value method github.com/ethersphere/bee/v2/pkg/log.marshalerTest.MarshalLog called using nil *marshalerTest pointer>"`,
	}, {
		val: marshalerPanicTest{"foobar"},
		exp: `"<panic: marshalerPanicTest>"`,
	}, {
		val: stringerTest{"foobar"},
		exp: `"I am a fmt.Stringer"`,
	}, {
		val: &stringerTest{"foobar"},
		exp: `"I am a fmt.Stringer"`,
	}, {
		val: (*stringerTest)(nil),
		exp: `"<panic: value method github.com/ethersphere/bee/v2/pkg/log.stringerTest.String called using nil *stringerTest pointer>"`,
	}, {
		val: stringerPanicTest{"foobar"},
		exp: `"<panic: stringerPanicTest>"`,
	}, {
		val: errorTest{"foobar"},
		exp: `"I am an error"`,
	}, {
		val: &errorTest{"foobar"},
		exp: `"I am an error"`,
	}, {
		val: (*errorTest)(nil),
		exp: `"<panic: value method github.com/ethersphere/bee/v2/pkg/log.errorTest.Error called using nil *errorTest pointer>"`,
	}, {
		val: errorPanicTest{"foobar"},
		exp: `"<panic: errorPanicTest>"`,
	}, {
		val: jsonTagsStringTest{
			String1: "v1",
			String2: "v2",
			String3: "v3",
			String4: "v4",
			String5: "v5",
			String6: "v6",
		},
	}, {
		val: jsonTagsStringTest{},
	}, {
		val: jsonTagsBoolTest{
			Bool1: true,
			Bool2: true,
			Bool3: true,
			Bool4: true,
			Bool5: true,
			Bool6: true,
		},
	}, {
		val: jsonTagsBoolTest{},
	}, {
		val: jsonTagsIntTest{
			Int1: 1,
			Int2: 2,
			Int3: 3,
			Int4: 4,
			Int5: 5,
			Int6: 6,
		},
	}, {
		val: jsonTagsIntTest{},
	}, {
		val: jsonTagsUintTest{
			Uint1: 1,
			Uint2: 2,
			Uint3: 3,
			Uint4: 4,
			Uint5: 5,
			Uint6: 6,
		},
	}, {
		val: jsonTagsUintTest{},
	}, {
		val: jsonTagsFloatTest{
			Float1: 1.1,
			Float2: 2.2,
			Float3: 3.3,
			Float4: 4.4,
			Float5: 5.5,
			Float6: 6.6,
		},
	}, {
		val: jsonTagsFloatTest{},
	}, {
		val: jsonTagsComplexTest{
			Complex1: 1i,
			Complex2: 2i,
			Complex3: 3i,
			Complex4: 4i,
			Complex5: 5i,
			Complex6: 6i,
		},
		exp: `{"complex1":"(0+1i)","-":"(0+3i)","complex4":"(0+4i)","Complex5":"(0+5i)","Complex6":"(0+6i)"}`,
	}, {
		val: jsonTagsComplexTest{},
		exp: `{"complex1":"(0+0i)","-":"(0+0i)","Complex5":"(0+0i)"}`,
	}, {
		val: jsonTagsPtrTest{
			Ptr1: strPtr("1"),
			Ptr2: strPtr("2"),
			Ptr3: strPtr("3"),
			Ptr4: strPtr("4"),
			Ptr5: strPtr("5"),
			Ptr6: strPtr("6"),
		},
	}, {
		val: jsonTagsPtrTest{},
	}, {
		val: jsonTagsArrayTest{
			Array1: [2]string{"v1", "v1"},
			Array2: [2]string{"v2", "v2"},
			Array3: [2]string{"v3", "v3"},
			Array4: [2]string{"v4", "v4"},
			Array5: [2]string{"v5", "v5"},
			Array6: [2]string{"v6", "v6"},
		},
	}, {
		val: jsonTagsArrayTest{},
	}, {
		val: jsonTagsSliceTest{
			Slice1: []string{"v1", "v1"},
			Slice2: []string{"v2", "v2"},
			Slice3: []string{"v3", "v3"},
			Slice4: []string{"v4", "v4"},
			Slice5: []string{"v5", "v5"},
			Slice6: []string{"v6", "v6"},
		},
	}, {
		val: jsonTagsSliceTest{},
		exp: `{"slice1":[],"-":[],"Slice5":[]}`,
	}, {
		val: jsonTagsMapTest{
			Map1: map[string]string{"k1": "v1"},
			Map2: map[string]string{"k2": "v2"},
			Map3: map[string]string{"k3": "v3"},
			Map4: map[string]string{"k4": "v4"},
			Map5: map[string]string{"k5": "v5"},
			Map6: map[string]string{"k6": "v6"},
		},
	}, {
		val: jsonTagsMapTest{},
		exp: `{"map1":{},"-":{},"Map5":{}}`,
	}, {
		val: embedStructTest{},
	}, {
		val: embedNonStructTest{},
		exp: `{"InnerIntTest":0,"InnerMapTest":{},"InnerSliceTest":[]}`,
	}, {
		val: embedJSONTagsTest{},
	}, {
		val: PseudoStruct(makeKV("f1", 1, "f2", true, "f3", []int{})),
		exp: `{"f1":1,"f2":true,"f3":[]}`,
	}, {
		val: map[jsonTagsStringTest]int{
			{String1: `"quoted"`, String4: `unquoted`}: 1,
		},
		exp: `{"{\"string1\":\"\\\"quoted\\\"\",\"-\":\"\",\"string4\":\"unquoted\",\"String5\":\"\"}":1}`,
	}, {
		val: map[jsonTagsIntTest]int{
			{Int1: 1, Int2: 2}: 3,
		},
		exp: `{"{\"int1\":1,\"-\":0,\"Int5\":0}":3}`,
	}, {
		val: map[[2]struct{ S string }]int{
			{{S: `"quoted"`}, {S: "unquoted"}}: 1,
		},
		exp: `{"[{\"S\":\"\\\"quoted\\\"\"},{\"S\":\"unquoted\"}]":1}`,
	}, {
		val: jsonTagsComplexTest{},
		exp: `{"complex1":"(0+0i)","-":"(0+0i)","Complex5":"(0+0i)"}`,
	}, {
		val: jsonTagsPtrTest{
			Ptr1: strPtr("1"),
			Ptr2: strPtr("2"),
			Ptr3: strPtr("3"),
			Ptr4: strPtr("4"),
			Ptr5: strPtr("5"),
			Ptr6: strPtr("6"),
		},
	}, {
		val: jsonTagsPtrTest{},
	}, {
		val: jsonTagsArrayTest{
			Array1: [2]string{"v1", "v1"},
			Array2: [2]string{"v2", "v2"},
			Array3: [2]string{"v3", "v3"},
			Array4: [2]string{"v4", "v4"},
			Array5: [2]string{"v5", "v5"},
			Array6: [2]string{"v6", "v6"},
		},
	}, {
		val: jsonTagsArrayTest{},
	}, {
		val: jsonTagsSliceTest{
			Slice1: []string{"v1", "v1"},
			Slice2: []string{"v2", "v2"},
			Slice3: []string{"v3", "v3"},
			Slice4: []string{"v4", "v4"},
			Slice5: []string{"v5", "v5"},
			Slice6: []string{"v6", "v6"},
		},
	}, {
		val: jsonTagsSliceTest{},
		exp: `{"slice1":[],"-":[],"Slice5":[]}`,
	}, {
		val: jsonTagsMapTest{
			Map1: map[string]string{"k1": "v1"},
			Map2: map[string]string{"k2": "v2"},
			Map3: map[string]string{"k3": "v3"},
			Map4: map[string]string{"k4": "v4"},
			Map5: map[string]string{"k5": "v5"},
			Map6: map[string]string{"k6": "v6"},
		},
	}, {
		val: jsonTagsMapTest{},
		exp: `{"map1":{},"-":{},"Map5":{}}`,
	}, {
		val: embedStructTest{},
	}, {
		val: embedNonStructTest{},
		exp: `{"InnerIntTest":0,"InnerMapTest":{},"InnerSliceTest":[]}`,
	}, {
		val: embedJSONTagsTest{},
	}, {
		val: PseudoStruct(makeKV("f1", 1, "f2", true, "f3", []int{})),
		exp: `{"f1":1,"f2":true,"f3":[]}`,
	}, {
		val: map[jsonTagsStringTest]int{
			{String1: `"quoted"`, String4: `unquoted`}: 1,
		},
		exp: `{"{\"string1\":\"\\\"quoted\\\"\",\"-\":\"\",\"string4\":\"unquoted\",\"String5\":\"\"}":1}`,
	}, {
		val: map[jsonTagsIntTest]int{
			{Int1: 1, Int2: 2}: 3,
		},
		exp: `{"{\"int1\":1,\"-\":0,\"Int5\":0}":3}`,
	}, {
		val: map[[2]struct{ S string }]int{
			{{S: `"quoted"`}, {S: "unquoted"}}: 1,
		},
		exp: `{"[{\"S\":\"\\\"quoted\\\"\"},{\"S\":\"unquoted\"}]":1}`,
	}}

	o := *defaults.options
	f := newFormatter(o.fmtOptions)
	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			var want string
			have := f.prettyWithFlags(tc.val, 0, 0)

			if tc.exp != "" {
				want = tc.exp
			} else {
				jb, err := json.Marshal(tc.val)
				if err != nil {
					t.Fatalf("unexpected error: %v\nhave: %q", err, have)
				}
				want = string(jb)
			}

			if have != want {
				t.Errorf("prettyWithFlags(...):\n\twant %q\n\thave %q", want, have)
			}
		})
	}
}

func makeKV(args ...interface{}) []interface{} { return args }

func TestRender(t *testing.T) {
	testCases := []struct {
		name     string
		builtins []interface{}
		args     []interface{}
		wantKV   string
		wantJSON string
	}{{
		name:     "nil",
		wantKV:   "",
		wantJSON: "{}",
	}, {
		name:     "empty",
		builtins: []interface{}{},
		args:     []interface{}{},
		wantKV:   "",
		wantJSON: "{}",
	}, {
		name:     "primitives",
		builtins: makeKV("int1", 1, "int2", 2),
		args:     makeKV("bool1", true, "bool2", false),
		wantKV:   `"int1"=1 "int2"=2 "bool1"=true "bool2"=false`,
		wantJSON: `{"int1":1,"int2":2,"bool1":true,"bool2":false}`,
	}, {
		name:     "pseudo structs",
		builtins: makeKV("int", PseudoStruct(makeKV("intsub", 1))),
		args:     makeKV("bool", PseudoStruct(makeKV("boolsub", true))),
		wantKV:   `"int"={"intsub":1} "bool"={"boolsub":true}`,
		wantJSON: `{"int":{"intsub":1},"bool":{"boolsub":true}}`,
	}, {
		name:     "escapes",
		builtins: makeKV("\"1\"", 1),     // will not be escaped, but should never happen
		args:     makeKV("bool\n", true), // escaped
		wantKV:   `""1""=1 "bool\n"=true`,
		wantJSON: `{""1"":1,"bool\n":true}`,
	}, {
		name:     "missing value",
		builtins: makeKV("builtin"),
		args:     makeKV("arg"),
		wantKV:   `"builtin"="<no-value>" "arg"="<no-value>"`,
		wantJSON: `{"builtin":"<no-value>","arg":"<no-value>"}`,
	}, {
		name:     "non-string key int",
		builtins: makeKV(123, "val"), // should never happen
		args:     makeKV(456, "val"),
		wantKV:   `"<non-string-key: 123>"="val" "<non-string-key: 456>"="val"`,
		wantJSON: `{"<non-string-key: 123>":"val","<non-string-key: 456>":"val"}`,
	}, {
		name: "non-string key struct",
		builtins: makeKV(struct { // will not be escaped, but should never happen
			F1 string
			F2 int
		}{"builtin", 123}, "val"),
		args: makeKV(struct {
			F1 string
			F2 int
		}{"arg", 456}, "val"),
		wantKV:   `"<non-string-key: {"F1":"builtin",>"="val" "<non-string-key: {\"F1\":\"arg\",\"F2\">"="val"`,
		wantJSON: `{"<non-string-key: {"F1":"builtin",>":"val","<non-string-key: {\"F1\":\"arg\",\"F2\">":"val"}`,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			test := func(t *testing.T, formatter *formatter, want string) {
				t.Helper()

				have := string(bytes.TrimRight(formatter.render(tc.builtins, tc.args), "\n"))
				if have != want {
					t.Errorf("render(...):\nwant %q\nhave %q", want, have)
				}
			}
			t.Run("KV", func(t *testing.T) {
				o := *defaults.options
				test(t, newFormatter(o.fmtOptions), tc.wantKV)
			})
			t.Run("JSON", func(t *testing.T) {
				o := *defaults.options
				WithJSONOutput()(&o)
				test(t, newFormatter(o.fmtOptions), tc.wantJSON)
			})
		})
	}
}

func TestSanitize(t *testing.T) {
	testCases := []struct {
		name string
		kv   []interface{}
		want []interface{}
	}{{
		name: "empty",
		kv:   []interface{}{},
		want: []interface{}{},
	}, {
		name: "already sane",
		kv:   makeKV("int", 1, "str", "ABC", "bool", true),
		want: makeKV("int", 1, "str", "ABC", "bool", true),
	}, {
		name: "missing value",
		kv:   makeKV("key"),
		want: makeKV("key", "<no-value>"),
	}, {
		name: "non-string key int",
		kv:   makeKV(123, "val"),
		want: makeKV("<non-string-key: 123>", "val"),
	}, {
		name: "non-string key struct",
		kv: makeKV(struct {
			F1 string
			F2 int
		}{"f1", 8675309}, "val"),
		want: makeKV(`<non-string-key: {"F1":"f1","F2":>`, "val"),
	}}

	o := *defaults.options
	WithJSONOutput()(&o)
	f := newFormatter(o.fmtOptions)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			have := f.sanitize(tc.kv)
			if diff := cmp.Diff(have, tc.want); diff != "" {
				t.Errorf("sanitize(...) mismatch (-want +have):\n%s", diff)
			}
		})
	}
}

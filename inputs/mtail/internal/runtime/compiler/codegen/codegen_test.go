// Copyright 2011 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package codegen_test

import (
	"flag"
	"strings"
	"testing"
	"time"

	"flashcat.cloud/categraf/inputs/mtail/internal/runtime/code"
	"flashcat.cloud/categraf/inputs/mtail/internal/runtime/compiler/ast"
	"flashcat.cloud/categraf/inputs/mtail/internal/runtime/compiler/checker"
	"flashcat.cloud/categraf/inputs/mtail/internal/runtime/compiler/codegen"
	"flashcat.cloud/categraf/inputs/mtail/internal/runtime/compiler/parser"
	"flashcat.cloud/categraf/inputs/mtail/internal/testutil"
)

var codegenTestDebug = flag.Bool("codegen_test_debug", false, "Log ASTs and debugging information ")

var testCodeGenPrograms = []struct {
	name   string
	source string
	prog   []code.Instr // expected bytecode
}{
	// Composite literals require too many explicit conversions.
	{
		name:   "simple line counter",
		source: "counter lines_total\n/$/ { lines_total++\n }\n",
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 7, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 1},
			{Opcode: code.Dload, Operand: 0, SourceLine: 1},
			{Opcode: code.Inc, Operand: nil, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name:   "count a",
		source: "counter a_count\n/a$/ { a_count++\n }\n",
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 7, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 1},
			{Opcode: code.Dload, Operand: 0, SourceLine: 1},
			{Opcode: code.Inc, Operand: nil, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "strptime and capref",
		source: "counter foo\n" +
			"/(.*)/ { strptime($1, \"2006-01-02T15:04:05\")\n" +
			"foo++\n}\n",
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 11, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Push, Operand: 0, SourceLine: 1},
			{Opcode: code.Capref, Operand: 1, SourceLine: 1},
			{Opcode: code.Str, Operand: 0, SourceLine: 1},
			{Opcode: code.Strptime, Operand: 2, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "strptime and named capref",
		source: "counter foo\n" +
			"/(?P<date>.*)/ { strptime($date, \"2006-01-02T15:04:05\")\n" +
			"foo++\n }\n",
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 11, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Push, Operand: 0, SourceLine: 1},
			{Opcode: code.Capref, Operand: 1, SourceLine: 1},
			{Opcode: code.Str, Operand: 0, SourceLine: 1},
			{Opcode: code.Strptime, Operand: 2, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "inc by and set",
		source: "counter foo\ncounter bar\n" +
			"/([0-9]+)/ {\n" +
			"foo += $1\n" +
			"bar = $1\n" +
			"}\n",
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 16, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.S2i, Operand: nil, SourceLine: 3},
			{Opcode: code.Inc, Operand: 0, SourceLine: 3},
			{Opcode: code.Mload, Operand: 1, SourceLine: 4},
			{Opcode: code.Dload, Operand: 0, SourceLine: 4},
			{Opcode: code.Push, Operand: 0, SourceLine: 4},
			{Opcode: code.Capref, Operand: 1, SourceLine: 4},
			{Opcode: code.S2i, Operand: nil, SourceLine: 4},
			{Opcode: code.Iset, Operand: nil, SourceLine: 4},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "cond expr gt",
		source: "counter foo\n" +
			"1 > 0 {\n" +
			"  foo++\n" +
			"}\n",
		prog: []code.Instr{
			{Opcode: code.Push, Operand: int64(1), SourceLine: 1},
			{Opcode: code.Push, Operand: int64(0), SourceLine: 1},
			{Opcode: code.Icmp, Operand: 1, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 6, SourceLine: 1},
			{Opcode: code.Push, Operand: true, SourceLine: 1},
			{Opcode: code.Jmp, Operand: 7, SourceLine: 1},
			{Opcode: code.Push, Operand: false, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "cond expr lt",
		source: "counter foo\n" +
			"1 < 0 {\n" +
			"  foo++\n" +
			"}\n",
		prog: []code.Instr{
			{Opcode: code.Push, Operand: int64(1), SourceLine: 1},
			{Opcode: code.Push, Operand: int64(0), SourceLine: 1},
			{Opcode: code.Icmp, Operand: -1, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 6, SourceLine: 1},
			{Opcode: code.Push, Operand: true, SourceLine: 1},
			{Opcode: code.Jmp, Operand: 7, SourceLine: 1},
			{Opcode: code.Push, Operand: false, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "cond expr eq",
		source: "counter foo\n" +
			"1 == 0 {\n" +
			"  foo++\n" +
			"}\n",
		prog: []code.Instr{
			{Opcode: code.Push, Operand: int64(1), SourceLine: 1},
			{Opcode: code.Push, Operand: int64(0), SourceLine: 1},
			{Opcode: code.Icmp, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 6, SourceLine: 1},
			{Opcode: code.Push, Operand: true, SourceLine: 1},
			{Opcode: code.Jmp, Operand: 7, SourceLine: 1},
			{Opcode: code.Push, Operand: false, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "cond expr le",
		source: "counter foo\n" +
			"1 <= 0 {\n" +
			"  foo++\n" +
			"}\n",
		prog: []code.Instr{
			{Opcode: code.Push, Operand: int64(1), SourceLine: 1},
			{Opcode: code.Push, Operand: int64(0), SourceLine: 1},
			{Opcode: code.Icmp, Operand: 1, SourceLine: 1},
			{Opcode: code.Jm, Operand: 6, SourceLine: 1},
			{Opcode: code.Push, Operand: true, SourceLine: 1},
			{Opcode: code.Jmp, Operand: 7, SourceLine: 1},
			{Opcode: code.Push, Operand: false, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "cond expr ge",
		source: "counter foo\n" +
			"1 >= 0 {\n" +
			"  foo++\n" +
			"}\n",
		prog: []code.Instr{
			{Opcode: code.Push, Operand: int64(1), SourceLine: 1},
			{Opcode: code.Push, Operand: int64(0), SourceLine: 1},
			{Opcode: code.Icmp, Operand: -1, SourceLine: 1},
			{Opcode: code.Jm, Operand: 6, SourceLine: 1},
			{Opcode: code.Push, Operand: true, SourceLine: 1},
			{Opcode: code.Jmp, Operand: 7, SourceLine: 1},
			{Opcode: code.Push, Operand: false, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "cond expr ne",
		source: "counter foo\n" +
			"1 != 0 {\n" +
			"  foo++\n" +
			"}\n",
		prog: []code.Instr{
			{Opcode: code.Push, Operand: int64(1), SourceLine: 1},
			{Opcode: code.Push, Operand: int64(0), SourceLine: 1},
			{Opcode: code.Icmp, Operand: 0, SourceLine: 1},
			{Opcode: code.Jm, Operand: 6, SourceLine: 1},
			{Opcode: code.Push, Operand: true, SourceLine: 1},
			{Opcode: code.Jmp, Operand: 7, SourceLine: 1},
			{Opcode: code.Push, Operand: false, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "nested cond",
		source: "counter foo\n" +
			"/(\\d+)/ {\n" +
			"  $1 <= 1 {\n" +
			"    foo++\n" +
			"  }\n" +
			"}\n",
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 19, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 1, SourceLine: 2},
			{Opcode: code.S2i, Operand: nil, SourceLine: 2},
			{Opcode: code.Push, Operand: int64(1), SourceLine: 2},
			{Opcode: code.Icmp, Operand: 1, SourceLine: 2},
			{Opcode: code.Jm, Operand: 11, SourceLine: 2},
			{Opcode: code.Push, Operand: true, SourceLine: 2},
			{Opcode: code.Jmp, Operand: 12, SourceLine: 2},
			{Opcode: code.Push, Operand: false, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 18, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Inc, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "deco",
		source: "counter foo\n" +
			"counter bar\n" +
			"def fooWrap {\n" +
			"  /.*/ {\n" +
			"    foo++\n" +
			"    next\n" +
			"  }\n" +
			"}\n" +
			"" +
			"@fooWrap { bar++\n }\n",
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 3},
			{Opcode: code.Jnm, Operand: 10, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 3},
			{Opcode: code.Mload, Operand: 0, SourceLine: 4},
			{Opcode: code.Dload, Operand: 0, SourceLine: 4},
			{Opcode: code.Inc, Operand: nil, SourceLine: 4},
			{Opcode: code.Mload, Operand: 1, SourceLine: 8},
			{Opcode: code.Dload, Operand: 0, SourceLine: 8},
			{Opcode: code.Inc, Operand: nil, SourceLine: 8},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 3},
		},
	},
	{
		name: "length",
		source: "len(\"foo\") > 0 {\n" +
			"}\n",
		prog: []code.Instr{
			{Opcode: code.Str, Operand: 0, SourceLine: 0},
			{Opcode: code.Length, Operand: 1, SourceLine: 0},
			{Opcode: code.Push, Operand: int64(0), SourceLine: 0},
			{Opcode: code.Cmp, Operand: 1, SourceLine: 0},
			{Opcode: code.Jnm, Operand: 7, SourceLine: 0},
			{Opcode: code.Push, Operand: true, SourceLine: 0},
			{Opcode: code.Jmp, Operand: 8, SourceLine: 0},
			{Opcode: code.Push, Operand: false, SourceLine: 0},
			{Opcode: code.Jnm, Operand: 11, SourceLine: 0},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 0},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 0},
		},
	},
	{
		name: "bitwise", source: `
gauge a

a = 1 & 7 ^ 15 | 8
a = ~ 16 << 2
a = 1 >> 20
`,
		prog: []code.Instr{
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Push, Operand: int64(1), SourceLine: 3},
			{Opcode: code.Push, Operand: int64(7), SourceLine: 3},
			{Opcode: code.And, Operand: nil, SourceLine: 3},
			{Opcode: code.Push, Operand: int64(15), SourceLine: 3},
			{Opcode: code.Xor, Operand: nil, SourceLine: 3},
			{Opcode: code.Push, Operand: int64(8), SourceLine: 3},
			{Opcode: code.Or, Operand: nil, SourceLine: 3},
			{Opcode: code.Iset, Operand: nil, SourceLine: 3},
			{Opcode: code.Mload, Operand: 0, SourceLine: 4},
			{Opcode: code.Dload, Operand: 0, SourceLine: 4},
			{Opcode: code.Push, Operand: int64(16), SourceLine: 4},
			{Opcode: code.Neg, Operand: nil, SourceLine: 4},
			{Opcode: code.Push, Operand: int64(2), SourceLine: 4},
			{Opcode: code.Shl, Operand: nil, SourceLine: 4},
			{Opcode: code.Iset, Operand: nil, SourceLine: 4},
			{Opcode: code.Mload, Operand: 0, SourceLine: 5},
			{Opcode: code.Dload, Operand: 0, SourceLine: 5},
			{Opcode: code.Push, Operand: int64(1), SourceLine: 5},
			{Opcode: code.Push, Operand: int64(20), SourceLine: 5},
			{Opcode: code.Shr, Operand: nil, SourceLine: 5},
			{Opcode: code.Iset, Operand: nil, SourceLine: 5},
		},
	},
	{
		name: "pow", source: `
gauge a
/(\d+) (\d+)/ {
  a = $1 ** $2
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 14, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.S2i, Operand: nil, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 2, SourceLine: 3},
			{Opcode: code.S2i, Operand: nil, SourceLine: 3},
			{Opcode: code.Ipow, Operand: nil, SourceLine: 3},
			{Opcode: code.Iset, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "indexed expr", source: `
counter a by b
a["string"]++
`,
		prog: []code.Instr{
			{Opcode: code.Str, Operand: 0, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 1, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
		},
	},
	{
		name: "strtol", source: `
strtol("deadbeef", 16)
`,
		prog: []code.Instr{
			{Opcode: code.Str, Operand: 0, SourceLine: 1},
			{Opcode: code.Push, Operand: int64(16), SourceLine: 1},
			{Opcode: code.S2i, Operand: 2, SourceLine: 1},
		},
	},
	{
		name: "float", source: `
20.0
`,
		prog: []code.Instr{
			{Opcode: code.Push, Operand: 20.0, SourceLine: 1},
		},
	},
	{
		name: "otherwise", source: `
counter a
otherwise {
	a++
}
`,
		prog: []code.Instr{
			{Opcode: code.Otherwise, Operand: nil, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 7, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Inc, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "cond else",
		source: `counter foo
counter bar
1 > 0 {
  foo++
} else {
  bar++
}`,
		prog: []code.Instr{
			{Opcode: code.Push, Operand: int64(1), SourceLine: 2},
			{Opcode: code.Push, Operand: int64(0), SourceLine: 2},
			{Opcode: code.Icmp, Operand: 1, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 6, SourceLine: 2},
			{Opcode: code.Push, Operand: true, SourceLine: 2},
			{Opcode: code.Jmp, Operand: 7, SourceLine: 2},
			{Opcode: code.Push, Operand: false, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 14, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Inc, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
			{Opcode: code.Jmp, Operand: 17, SourceLine: 2},
			{Opcode: code.Mload, Operand: 1, SourceLine: 5},
			{Opcode: code.Dload, Operand: 0, SourceLine: 5},
			{Opcode: code.Inc, Operand: nil, SourceLine: 5},
		},
	},
	{
		name: "mod",
		source: `
gauge a
a = 3 % 1
`,
		prog: []code.Instr{
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Push, Operand: int64(3), SourceLine: 2},
			{Opcode: code.Push, Operand: int64(1), SourceLine: 2},
			{Opcode: code.Imod, Operand: nil, SourceLine: 2},
			{Opcode: code.Iset, Operand: nil, SourceLine: 2},
		},
	},
	{
		name: "del", source: `
counter a by b
del a["string"]
`,
		prog: []code.Instr{
			{Opcode: code.Str, Operand: 0, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Del, Operand: 1, SourceLine: 2},
		},
	},
	{
		name: "del after", source: `
counter a by b
del a["string"] after 1h
`,
		prog: []code.Instr{
			{Opcode: code.Push, Operand: time.Hour, SourceLine: 2},
			{Opcode: code.Str, Operand: 0, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Expire, Operand: 1, SourceLine: 2},
		},
	},
	{
		name: "types", source: `
gauge i
gauge f
/(\d+)/ {
 i = $1
}
/(\d+\.\d+)/ {
 f = $1
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 3},
			{Opcode: code.Jnm, Operand: 10, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 3},
			{Opcode: code.Mload, Operand: 0, SourceLine: 4},
			{Opcode: code.Dload, Operand: 0, SourceLine: 4},
			{Opcode: code.Push, Operand: 0, SourceLine: 4},
			{Opcode: code.Capref, Operand: 1, SourceLine: 4},
			{Opcode: code.S2i, Operand: nil, SourceLine: 4},
			{Opcode: code.Iset, Operand: nil, SourceLine: 4},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 3},
			{Opcode: code.Match, Operand: 1, SourceLine: 6},
			{Opcode: code.Jnm, Operand: 20, SourceLine: 6},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 6},
			{Opcode: code.Mload, Operand: 1, SourceLine: 7},
			{Opcode: code.Dload, Operand: 0, SourceLine: 7},
			{Opcode: code.Push, Operand: 1, SourceLine: 7},
			{Opcode: code.Capref, Operand: 1, SourceLine: 7},
			{Opcode: code.S2f, Operand: nil, SourceLine: 7},
			{Opcode: code.Fset, Operand: nil, SourceLine: 7},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 6},
		},
	},

	{
		name: "getfilename", source: `
getfilename()
`,
		prog: []code.Instr{
			{Opcode: code.Getfilename, Operand: 0, SourceLine: 1},
		},
	},

	{
		name: "dimensioned counter",
		source: `counter c by a,b,c
/(\d) (\d) (\d)/ {
  c[$1,$2][$3]++
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 1, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 2, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 3, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 3, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "string to int",
		source: `counter c
/(.*)/ {
  c = int($1)
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 10, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 1, SourceLine: 2},
			{Opcode: code.S2i, Operand: nil, SourceLine: 2},
			{Opcode: code.Iset, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "int to float",
		source: `counter c
/(\d)/ {
  c = float($1)
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 10, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 1, SourceLine: 2},
			{Opcode: code.S2f, Operand: nil, SourceLine: 2},
			{Opcode: code.Fset, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "string to float",
		source: `counter c
/(.*)/ {
  c = float($1)
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 10, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 0, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 1, SourceLine: 2},
			{Opcode: code.S2f, Operand: nil, SourceLine: 2},
			{Opcode: code.Fset, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "float to string",
		source: `counter c by a
/(\d+\.\d+)/ {
  c[string($1)] ++
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 11, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 1, SourceLine: 2},
			{Opcode: code.S2f, Operand: nil, SourceLine: 2},
			{Opcode: code.F2s, Operand: nil, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 1, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "int to string",
		source: `counter c by a
/(\d+)/ {
  c[string($1)] ++
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 11, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 1, SourceLine: 2},
			{Opcode: code.S2i, Operand: nil, SourceLine: 2},
			{Opcode: code.I2s, Operand: nil, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 2},
			{Opcode: code.Dload, Operand: 1, SourceLine: 2},
			{Opcode: code.Inc, Operand: nil, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "nested comparisons",
		source: `counter foo
/(.*)/ {
  $1 == "foo" || $1 == "bar" {
    foo++
  }
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 31, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 1, SourceLine: 2},
			{Opcode: code.Str, Operand: 0, SourceLine: 2},
			{Opcode: code.Scmp, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 10, SourceLine: 2},
			{Opcode: code.Push, Operand: true, SourceLine: 2},
			{Opcode: code.Jmp, Operand: 11, SourceLine: 2},
			{Opcode: code.Push, Operand: false, SourceLine: 2},
			{Opcode: code.Jm, Operand: 23, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 2},
			{Opcode: code.Capref, Operand: 1, SourceLine: 2},
			{Opcode: code.Str, Operand: 1, SourceLine: 2},
			{Opcode: code.Scmp, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 19, SourceLine: 2},
			{Opcode: code.Push, Operand: true, SourceLine: 2},
			{Opcode: code.Jmp, Operand: 20, SourceLine: 2},
			{Opcode: code.Push, Operand: false, SourceLine: 2},
			{Opcode: code.Jm, Operand: 23, SourceLine: 2},
			{Opcode: code.Push, Operand: false, SourceLine: 2},
			{Opcode: code.Jmp, Operand: 24, SourceLine: 2},
			{Opcode: code.Push, Operand: true, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 30, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Inc, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "string concat", source: `
counter f by s
/(.*), (.*)/ {
  f[$1 + $2]++
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 12, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 2, SourceLine: 3},
			{Opcode: code.Cat, Operand: nil, SourceLine: 3},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 1, SourceLine: 3},
			{Opcode: code.Inc, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "add assign float", source: `
gauge foo
/(\d+\.\d+)/ {
  foo += $1
}
`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.S2f, Operand: nil, SourceLine: 3},
			{Opcode: code.Fadd, Operand: nil, SourceLine: 3},
			{Opcode: code.Fset, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "match expression", source: `
	counter foo
	/(.*)/ {
	  $1 =~ /asdf/ {
	    foo++
	  }
	}`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.Smatch, Operand: 1, SourceLine: 3},
			{Opcode: code.Jnm, Operand: 12, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 3},
			{Opcode: code.Mload, Operand: 0, SourceLine: 4},
			{Opcode: code.Dload, Operand: 0, SourceLine: 4},
			{Opcode: code.Inc, Operand: nil, SourceLine: 4},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "negative match expression", source: `
	counter foo
	/(.*)/ {
	  $1 !~ /asdf/ {
	    foo++
	  }
	}`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 14, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.Smatch, Operand: 1, SourceLine: 3},
			{Opcode: code.Not, Operand: nil, SourceLine: 3},
			{Opcode: code.Jnm, Operand: 13, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 3},
			{Opcode: code.Mload, Operand: 0, SourceLine: 4},
			{Opcode: code.Dload, Operand: 0, SourceLine: 4},
			{Opcode: code.Inc, Operand: nil, SourceLine: 4},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "capref used in def", source: `
/(?P<x>\d+)/ && $x > 5 {
}`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 14, SourceLine: 1},
			{Opcode: code.Push, Operand: 0, SourceLine: 1},
			{Opcode: code.Capref, Operand: 1, SourceLine: 1},
			{Opcode: code.S2i, Operand: nil, SourceLine: 1},
			{Opcode: code.Push, Operand: int64(5), SourceLine: 1},
			{Opcode: code.Icmp, Operand: 1, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 10, SourceLine: 1},
			{Opcode: code.Push, Operand: true, SourceLine: 1},
			{Opcode: code.Jmp, Operand: 11, SourceLine: 1},
			{Opcode: code.Push, Operand: false, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 14, SourceLine: 1},
			{Opcode: code.Push, Operand: true, SourceLine: 1},
			{Opcode: code.Jmp, Operand: 15, SourceLine: 1},
			{Opcode: code.Push, Operand: false, SourceLine: 1},
			{Opcode: code.Jnm, Operand: 18, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
		},
	},
	{
		name: "binop arith type conversion", source: `
gauge var
/(?P<x>\d+) (\d+\.\d+)/ {
  var = $x + $2
}`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 15, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.S2i, Operand: nil, SourceLine: 3},
			{Opcode: code.I2f, Operand: nil, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 2, SourceLine: 3},
			{Opcode: code.S2f, Operand: nil, SourceLine: 3},
			{Opcode: code.Fadd, Operand: nil, SourceLine: 3},
			{Opcode: code.Fset, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "binop compare type conversion", source: `
counter var
/(?P<x>\d+) (\d+\.\d+)/ {
  $x > $2 {
    var++
  }
}`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 22, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.S2i, Operand: nil, SourceLine: 3},
			{Opcode: code.I2f, Operand: nil, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 2, SourceLine: 3},
			{Opcode: code.S2f, Operand: nil, SourceLine: 3},
			{Opcode: code.Fcmp, Operand: 1, SourceLine: 3},
			{Opcode: code.Jnm, Operand: 14, SourceLine: 3},
			{Opcode: code.Push, Operand: true, SourceLine: 3},
			{Opcode: code.Jmp, Operand: 15, SourceLine: 3},
			{Opcode: code.Push, Operand: false, SourceLine: 3},
			{Opcode: code.Jnm, Operand: 21, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 3},
			{Opcode: code.Mload, Operand: 0, SourceLine: 4},
			{Opcode: code.Dload, Operand: 0, SourceLine: 4},
			{Opcode: code.Inc, Operand: nil, SourceLine: 4},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "set string", source: `
text foo
/(.*)/ {
  foo = $1
}
`, prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 9, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.Sset, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		}},
	{
		name: "concat to text", source: `
text foo
/(?P<v>.*)/ {
		foo += $v
}`,
		prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 12, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Push, Operand: 0, SourceLine: 3},
			{Opcode: code.Capref, Operand: 1, SourceLine: 3},
			{Opcode: code.Cat, Operand: nil, SourceLine: 3},
			{Opcode: code.Sset, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		},
	},
	{
		name: "decrement", source: `
counter i
// {
  i--
}`, prog: []code.Instr{
			{Opcode: code.Match, Operand: 0, SourceLine: 2},
			{Opcode: code.Jnm, Operand: 7, SourceLine: 2},
			{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
			{Opcode: code.Mload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dload, Operand: 0, SourceLine: 3},
			{Opcode: code.Dec, Operand: nil, SourceLine: 3},
			{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
		}},
	{name: "capref and settime", source: `
/(\d+)/ {
  settime($1)
}`, prog: []code.Instr{
		{Opcode: code.Match, Operand: 0, SourceLine: 1},
		{Opcode: code.Jnm, Operand: 8, SourceLine: 1},
		{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
		{Opcode: code.Push, Operand: 0, SourceLine: 2},
		{Opcode: code.Capref, Operand: 1, SourceLine: 2},
		{Opcode: code.S2i, Operand: nil, SourceLine: 2},
		{Opcode: code.Settime, Operand: 1, SourceLine: 2},
		{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
	}},
	{name: "cast to self", source: `
/(\d+)/ {
settime(int($1))
}`, prog: []code.Instr{
		{Opcode: code.Match, Operand: 0, SourceLine: 1},
		{Opcode: code.Jnm, Operand: 8, SourceLine: 1},
		{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
		{Opcode: code.Push, Operand: 0, SourceLine: 2},
		{Opcode: code.Capref, Operand: 1, SourceLine: 2},
		{Opcode: code.S2i, Operand: nil, SourceLine: 2},
		{Opcode: code.Settime, Operand: 1, SourceLine: 2},
		{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
	}},
	{name: "stop", source: `
stop
`, prog: []code.Instr{
		{Opcode: code.Stop, Operand: nil, SourceLine: 1},
	}},
	{name: "stop inside", source: `
// {
stop
}
`, prog: []code.Instr{
		{Opcode: code.Match, Operand: 0, SourceLine: 1},
		{Opcode: code.Jnm, Operand: 5, SourceLine: 1},
		{Opcode: code.Setmatched, Operand: false, SourceLine: 1},
		{Opcode: code.Stop, Operand: nil, SourceLine: 2},
		{Opcode: code.Setmatched, Operand: true, SourceLine: 1},
	}},

	{
		name: "nested decorators",
		source: `def b {
  def b {
    next
  }
  @b {
    next
  }
}
@b {
}`, prog: nil,
	},
	{name: "negative numbers in capture groups", source: `
gauge foo
/(?P<value_ms>-?\d+)/ {
foo += $value_ms / 1000.0
}`, prog: []code.Instr{
		{Opcode: code.Match, Operand: 0, SourceLine: 2},
		{Opcode: code.Jnm, Operand: 16, SourceLine: 2},
		{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
		{Opcode: code.Mload, Operand: 0, SourceLine: 3},
		{Opcode: code.Dload, Operand: 0, SourceLine: 3},
		{Opcode: code.Mload, Operand: 0, SourceLine: 3},
		{Opcode: code.Dload, Operand: 0, SourceLine: 3},
		{Opcode: code.Push, Operand: 0, SourceLine: 3},
		{Opcode: code.Capref, Operand: 1, SourceLine: 3},
		{Opcode: code.S2i, Operand: nil, SourceLine: 3},
		{Opcode: code.I2f, Operand: nil, SourceLine: 3},
		{Opcode: code.Push, Operand: 1000.0, SourceLine: 3},
		{Opcode: code.Fdiv, Operand: nil, SourceLine: 3},
		{Opcode: code.Fadd, Operand: nil, SourceLine: 3},
		{Opcode: code.Fset, Operand: nil, SourceLine: 3},
		{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
	}},
	{name: "substitution", source: `
gauge foo
/(\d+,\d)/ {
  foo = int(subst(",", "", $1))
}`, prog: []code.Instr{
		{Opcode: code.Match, Operand: 0, SourceLine: 2},
		{Opcode: code.Jnm, Operand: 13, SourceLine: 2},
		{Opcode: code.Setmatched, Operand: false, SourceLine: 2},
		{Opcode: code.Mload, Operand: 0, SourceLine: 3},
		{Opcode: code.Dload, Operand: 0, SourceLine: 3},
		{Opcode: code.Str, Operand: 0, SourceLine: 3},
		{Opcode: code.Str, Operand: 1, SourceLine: 3},
		{Opcode: code.Push, Operand: 0, SourceLine: 3},
		{Opcode: code.Capref, Operand: 1, SourceLine: 3},
		{Opcode: code.Subst, Operand: 3, SourceLine: 3},
		{Opcode: code.S2i, Operand: nil, SourceLine: 3},
		{Opcode: code.Iset, Operand: nil, SourceLine: 3},
		{Opcode: code.Setmatched, Operand: true, SourceLine: 2},
	}},
	{name: "const term as pattern", source: `
const A /n/
A && 1 {
}
`, prog: []code.Instr{
		{Opcode: code.Match, Operand: 0, SourceLine: 0},
		{Opcode: code.Jnm, Operand: 6, SourceLine: 0},
		{Opcode: code.Push, Operand: int64(1), SourceLine: 2},
		{Opcode: code.Jnm, Operand: 6, SourceLine: 0},
		{Opcode: code.Push, Operand: true, SourceLine: 0},
		{Opcode: code.Jmp, Operand: 7, SourceLine: 0},
		{Opcode: code.Push, Operand: false, SourceLine: 0},
		{Opcode: code.Jnm, Operand: 10, SourceLine: 0},
		{Opcode: code.Setmatched, Operand: false, SourceLine: 0},
		{Opcode: code.Setmatched, Operand: true, SourceLine: 0},
	}},
}

func TestCodeGenFromSource(t *testing.T) {
	for _, tc := range testCodeGenPrograms {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ast, err := parser.Parse(tc.name, strings.NewReader(tc.source))
			testutil.FatalIfErr(t, err)
			ast, err = checker.Check(ast, 0, 0)
			if *codegenTestDebug {
				s := parser.Sexp{}
				s.EmitTypes = true
				t.Log("Typed AST:\n" + s.Dump(ast))
			}
			testutil.FatalIfErr(t, err)
			obj, err := codegen.CodeGen(tc.name, ast)
			testutil.FatalIfErr(t, err)

			testutil.ExpectNoDiff(t, tc.prog, obj.Program, testutil.AllowUnexported(code.Instr{}))
		})
	}
}

var testCodeGenASTs = []struct {
	name string
	ast  ast.Node     // partial AST to be converted to bytecode
	prog []code.Instr // expected bytecode
}{
	{
		name: "subst",
		ast: &ast.BuiltinExpr{
			Name: "subst",
			Args: &ast.ExprList{
				Children: []ast.Node{
					&ast.StringLit{
						Text: "old",
					},
					&ast.StringLit{
						Text: "new",
					},
					&ast.StringLit{
						Text: "value",
					},
				},
			},
		},
		prog: []code.Instr{
			{Opcode: code.Str, Operand: 0, SourceLine: 0},
			{Opcode: code.Str, Operand: 1, SourceLine: 0},
			{Opcode: code.Str, Operand: 2, SourceLine: 0},
			{Opcode: code.Subst, Operand: 3, SourceLine: 0},
		},
	},
	{
		name: "regexp subst",
		ast: &ast.BuiltinExpr{
			Name: "subst",
			Args: &ast.ExprList{
				Children: []ast.Node{
					&ast.PatternExpr{
						Pattern: "a+",
						Expr: &ast.PatternLit{
							Pattern: "a+",
						},
					},
					&ast.StringLit{
						Text: "b",
					},
					&ast.StringLit{
						Text: "aaaaaa",
					},
				},
			},
		},
		prog: []code.Instr{
			{Opcode: code.Str, Operand: 0, SourceLine: 0},
			{Opcode: code.Str, Operand: 1, SourceLine: 0},
			{Opcode: code.Push, Operand: 0, SourceLine: 0},
			{Opcode: code.Rsubst, Operand: 3, SourceLine: 0},
		},
	},
}

func TestCodeGenFromAST(t *testing.T) {
	for _, tc := range testCodeGenASTs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			obj, err := codegen.CodeGen(tc.name, tc.ast)
			testutil.FatalIfErr(t, err)
			testutil.ExpectNoDiff(t, tc.prog, obj.Program, testutil.AllowUnexported(code.Instr{}))
		})
	}
}

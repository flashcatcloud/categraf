// Copyright 2011 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package vm

import (
	"context"
	"regexp"
	"testing"
	"time"

	"flashcat.cloud/categraf/inputs/mtail/internal/logline"
	"flashcat.cloud/categraf/inputs/mtail/internal/metrics"
	"flashcat.cloud/categraf/inputs/mtail/internal/metrics/datum"
	"flashcat.cloud/categraf/inputs/mtail/internal/runtime/code"
	"flashcat.cloud/categraf/inputs/mtail/internal/testutil"
)

var instructions = []struct {
	name          string
	i             code.Instr
	re            []*regexp.Regexp
	str           []string
	reversedStack []interface{} // stack is inverted to be pushed onto vm stack

	expectedStack  []interface{}
	expectedThread thread
}{
	{
		name:           "match",
		i:              code.Instr{Opcode: code.Match, Operand: 0},
		re:             []*regexp.Regexp{regexp.MustCompile("a*b")},
		str:            []string{},
		reversedStack:  []interface{}{},
		expectedStack:  []interface{}{true},
		expectedThread: thread{pc: 0, matches: map[int][]string{0: {"aaaab"}}},
	},
	{
		name:           "cmp lt",
		i:              code.Instr{Opcode: code.Cmp, Operand: -1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1, "2"},
		expectedStack:  []interface{}{true},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp eq",
		i:              code.Instr{Opcode: code.Cmp, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"2", "2"},
		expectedStack:  []interface{}{true},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp gt",
		i:              code.Instr{Opcode: code.Cmp, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 1},
		expectedStack:  []interface{}{true},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp le",
		i:              code.Instr{Opcode: code.Cmp, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, "2"},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp ne",
		i:              code.Instr{Opcode: code.Cmp, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"1", "2"},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp ge",
		i:              code.Instr{Opcode: code.Cmp, Operand: -1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 2},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp gt float float",
		i:              code.Instr{Opcode: code.Cmp, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"2.0", "1.0"},
		expectedStack:  []interface{}{true},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp gt float int",
		i:              code.Instr{Opcode: code.Cmp, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"1.0", "2"},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp gt int float",
		i:              code.Instr{Opcode: code.Cmp, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"1", "2.0"},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp eq string string false",
		i:              code.Instr{Opcode: code.Cmp, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"abc", "def"},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp eq string string true",
		i:              code.Instr{Opcode: code.Cmp, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"abc", "abc"},
		expectedStack:  []interface{}{true},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp gt float float",
		i:              code.Instr{Opcode: code.Cmp, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2.0, 1.0},
		expectedStack:  []interface{}{true},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp gt float int",
		i:              code.Instr{Opcode: code.Cmp, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1.0, 2},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cmp gt int float",
		i:              code.Instr{Opcode: code.Cmp, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1, 2.0},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "jnm",
		i:              code.Instr{Opcode: code.Jnm, Operand: 37},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{false},
		expectedStack:  []interface{}{},
		expectedThread: thread{pc: 37, matches: map[int][]string{}},
	},
	{
		name:           "jm",
		i:              code.Instr{Opcode: code.Jm, Operand: 37},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{false},
		expectedStack:  []interface{}{},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "jmp",
		i:              code.Instr{Opcode: code.Jmp, Operand: 37},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{},
		expectedStack:  []interface{}{},
		expectedThread: thread{pc: 37, matches: map[int][]string{}},
	},
	{
		name:          "strptime",
		i:             code.Instr{Opcode: code.Strptime, Operand: 0},
		re:            []*regexp.Regexp{},
		str:           []string{},
		reversedStack: []interface{}{"2012/01/18 06:25:00", "2006/01/02 15:04:05"},
		expectedStack: []interface{}{},
		expectedThread: thread{
			pc: 0, time: time.Date(2012, 1, 18, 6, 25, 0, 0, time.UTC),
			matches: map[int][]string{},
		},
	},
	{
		name:           "iadd",
		i:              code.Instr{Opcode: code.Iadd, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 1},
		expectedStack:  []interface{}{int64(3)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "isub",
		i:              code.Instr{Opcode: code.Isub, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 1},
		expectedStack:  []interface{}{int64(1)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "imul",
		i:              code.Instr{Opcode: code.Imul, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 1},
		expectedStack:  []interface{}{int64(2)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "idiv",
		i:              code.Instr{Opcode: code.Idiv, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{4, 2},
		expectedStack:  []interface{}{int64(2)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "imod",
		i:              code.Instr{Opcode: code.Imod, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{4, 2},
		expectedStack:  []interface{}{int64(0)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "imod 2",
		i:              code.Instr{Opcode: code.Imod, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{3, 2},
		expectedStack:  []interface{}{int64(1)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "tolower",
		i:              code.Instr{Opcode: code.Tolower, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"mIxeDCasE"},
		expectedStack:  []interface{}{"mixedcase"},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "length",
		i:              code.Instr{Opcode: code.Length, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"1234"},
		expectedStack:  []interface{}{4},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "length 0",
		i:              code.Instr{Opcode: code.Length, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{""},
		expectedStack:  []interface{}{0},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "shl",
		i:              code.Instr{Opcode: code.Shl, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 1},
		expectedStack:  []interface{}{int64(4)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "shr",
		i:              code.Instr{Opcode: code.Shr, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 1},
		expectedStack:  []interface{}{int64(1)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "and",
		i:              code.Instr{Opcode: code.And, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 1},
		expectedStack:  []interface{}{int64(0)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "or",
		i:              code.Instr{Opcode: code.Or, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 1},
		expectedStack:  []interface{}{int64(3)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "xor",
		i:              code.Instr{Opcode: code.Xor, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 1},
		expectedStack:  []interface{}{int64(3)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "xor 2",
		i:              code.Instr{Opcode: code.Xor, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 3},
		expectedStack:  []interface{}{int64(1)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "xor 3",
		i:              code.Instr{Opcode: code.Xor, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{-1, 3},
		expectedStack:  []interface{}{int64(^3)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "neg",
		i:              code.Instr{Opcode: code.Neg, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{0},
		expectedStack:  []interface{}{int64(-1)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "not",
		i:              code.Instr{Opcode: code.Not, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{false},
		expectedStack:  []interface{}{true},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "pow",
		i:              code.Instr{Opcode: code.Ipow, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2, 2},
		expectedStack:  []interface{}{int64(4)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "s2i pop",
		i:              code.Instr{Opcode: code.S2i, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"ff", 16},
		expectedStack:  []interface{}{int64(255)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "s2i",
		i:              code.Instr{Opcode: code.S2i},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"190"},
		expectedStack:  []interface{}{int64(190)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "s2f",
		i:              code.Instr{Opcode: code.S2f},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"1.0"},
		expectedStack:  []interface{}{float64(1.0)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "i2f",
		i:              code.Instr{Opcode: code.I2f},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1},
		expectedStack:  []interface{}{float64(1.0)},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "settime",
		i:              code.Instr{Opcode: code.Settime, Operand: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{int64(0)},
		expectedStack:  []interface{}{},
		expectedThread: thread{pc: 0, time: time.Unix(0, 0).UTC(), matches: map[int][]string{}},
	},
	{
		name:           "push int",
		i:              code.Instr{Opcode: code.Push, Operand: 1},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{},
		expectedStack:  []interface{}{1},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "push float",
		i:              code.Instr{Opcode: code.Push, Operand: 1.0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{},
		expectedStack:  []interface{}{1.0},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "setmatched false",
		i:              code.Instr{Opcode: code.Setmatched, Operand: false},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{},
		expectedStack:  []interface{}{},
		expectedThread: thread{matched: false, pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "setmatched true",
		i:              code.Instr{Opcode: code.Setmatched, Operand: true},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{},
		expectedStack:  []interface{}{},
		expectedThread: thread{matched: true, pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "otherwise",
		i:              code.Instr{Opcode: code.Otherwise},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{},
		expectedStack:  []interface{}{true},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "fadd",
		i:              code.Instr{Opcode: code.Fadd},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1.0, 2.0},
		expectedStack:  []interface{}{3.0},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "fsub",
		i:              code.Instr{Opcode: code.Fsub},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1.0, 2.0},
		expectedStack:  []interface{}{-1.0},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "fmul",
		i:              code.Instr{Opcode: code.Fmul},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1.0, 2.0},
		expectedStack:  []interface{}{2.0},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "fdiv",
		i:              code.Instr{Opcode: code.Fdiv},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1.0, 2.0},
		expectedStack:  []interface{}{0.5},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "fmod",
		i:              code.Instr{Opcode: code.Fmod},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1.0, 2.0},
		expectedStack:  []interface{}{1.0},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "fpow",
		i:              code.Instr{Opcode: code.Fpow},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{2.0, 2.0},
		expectedStack:  []interface{}{4.0},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "getfilename",
		i:              code.Instr{Opcode: code.Getfilename},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{},
		expectedStack:  []interface{}{testFilename},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "i2s",
		i:              code.Instr{Opcode: code.I2s},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1},
		expectedStack:  []interface{}{"1"},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "f2s",
		i:              code.Instr{Opcode: code.F2s},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{3.1},
		expectedStack:  []interface{}{"3.1"},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "cat",
		i:              code.Instr{Opcode: code.Cat, Operand: 0, SourceLine: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"first", "second"},
		expectedStack:  []interface{}{"firstsecond"},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "icmp gt false",
		i:              code.Instr{Opcode: code.Icmp, Operand: 1, SourceLine: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1, 2},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "fcmp gt false",
		i:              code.Instr{Opcode: code.Fcmp, Operand: 1, SourceLine: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{1.0, 2.0},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "scmp eq false",
		i:              code.Instr{Opcode: code.Scmp, Operand: 0, SourceLine: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"abc", "def"},
		expectedStack:  []interface{}{false},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
	{
		name:           "subst",
		i:              code.Instr{Opcode: code.Subst, Operand: 0, SourceLine: 0},
		re:             []*regexp.Regexp{},
		str:            []string{},
		reversedStack:  []interface{}{"aa" /*old*/, "a" /*new*/, "caat"},
		expectedStack:  []interface{}{"cat"},
		expectedThread: thread{pc: 0, matches: map[int][]string{}},
	},
}

const testFilename = "test"

// Testcode.Instrs tests that each instruction behaves as expected through one
// instruction cycle.
func TestInstrs(t *testing.T) {
	for _, tc := range instructions {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var m []*metrics.Metric
			m = append(m,
				metrics.NewMetric("foo", "test", metrics.Counter, metrics.Int),
				metrics.NewMetric("bar", "test", metrics.Counter, metrics.Int),
				metrics.NewMetric("quux", "test", metrics.Gauge, metrics.Float))
			obj := &code.Object{Regexps: tc.re, Strings: tc.str, Metrics: m, Program: []code.Instr{tc.i}}
			v := New(tc.name, obj, true, nil, false, false)
			v.t = new(thread)
			v.t.stack = make([]interface{}, 0)
			for _, item := range tc.reversedStack {
				v.t.Push(item)
			}
			v.t.matches = make(map[int][]string)
			v.input = logline.New(context.Background(), testFilename, "aaaab")
			v.execute(v.t, tc.i)
			if v.terminate {
				t.Fatalf("Execution failed, see info log.")
			}

			testutil.ExpectNoDiff(t, tc.expectedStack, v.t.stack)

			tc.expectedThread.stack = tc.expectedStack

			testutil.ExpectNoDiff(t, &tc.expectedThread, v.t, testutil.AllowUnexported(thread{}))
		})
	}
}

// makeVM is a helper method for construction a single-instruction VM.
func makeVM(i code.Instr, m []*metrics.Metric) *VM {
	obj := &code.Object{Metrics: m, Program: []code.Instr{i}}
	v := New("test", obj, true, nil, false, false)
	v.t = new(thread)
	v.t.stack = make([]interface{}, 0)
	v.t.matches = make(map[int][]string)
	v.input = logline.New(context.Background(), testFilename, "aaaab")
	return v
}

// makeMetrics returns a few useful metrics for observing under test.
func makeMetrics() []*metrics.Metric {
	var m []*metrics.Metric
	m = append(m,
		metrics.NewMetric("a", "tst", metrics.Counter, metrics.Int),
		metrics.NewMetric("b", "tst", metrics.Counter, metrics.Float),
		metrics.NewMetric("c", "tst", metrics.Gauge, metrics.String),
		metrics.NewMetric("d", "tst", metrics.Histogram, metrics.Float),
	)
	return m
}

type datumStoreTests struct {
	name     string
	i        code.Instr
	d        int // index of a metric in makeMetrics
	setup    func(t *thread, d datum.Datum)
	expected string
}

// code.Instructions with datum store side effects.
func TestDatumSetInstrs(t *testing.T) {
	tests := []datumStoreTests{
		{
			name: "simple inc",
			i:    code.Instr{Opcode: code.Inc},
			d:    0,
			setup: func(t *thread, d datum.Datum) {
				t.Push(d)
			},
			expected: "1",
		},
		{
			name: "inc by int",
			i:    code.Instr{Opcode: code.Inc, Operand: 0},
			d:    0,
			setup: func(t *thread, d datum.Datum) {
				t.Push(d)
				t.Push(2)
			},
			expected: "2",
		},
		{
			name: "inc by str",
			i:    code.Instr{Opcode: code.Inc, Operand: 0},
			d:    0,
			setup: func(t *thread, d datum.Datum) {
				t.Push(d)
				t.Push("4")
			},
			expected: "4",
		},
		{
			name: "iset",
			i:    code.Instr{Opcode: code.Iset},
			d:    0,
			setup: func(t *thread, d datum.Datum) {
				t.Push(d)
				t.Push(2)
			},
			expected: "2",
		},
		{
			name: "iset str",
			i:    code.Instr{Opcode: code.Iset},
			d:    0,
			setup: func(t *thread, d datum.Datum) {
				t.Push(d)
				t.Push("3")
			},
			expected: "3",
		},
		{
			name: "fset",
			i:    code.Instr{Opcode: code.Fset},
			d:    1,
			setup: func(t *thread, d datum.Datum) {
				t.Push(d)
				t.Push(3.1)
			},
			expected: "3.1",
		},
		{
			name: "fset str",
			i:    code.Instr{Opcode: code.Fset},
			d:    1,
			setup: func(t *thread, d datum.Datum) {
				t.Push(d)
				t.Push("4.1")
			},
			expected: "4.1",
		},
		{
			name: "sset",
			i:    code.Instr{Opcode: code.Sset},
			d:    2,
			setup: func(t *thread, d datum.Datum) {
				t.Push(d)
				t.Push("4.1")
			},
			expected: "4.1",
		},
		{
			name: "dec",
			i:    code.Instr{Opcode: code.Dec},
			d:    0,
			setup: func(t *thread, d datum.Datum) {
				datum.SetInt(d, 1, time.Now())
				t.Push(d)
			},
			expected: "0",
		},
		{
			name: "set hist",
			i:    code.Instr{Opcode: code.Fset},
			d:    3,
			setup: func(t *thread, d datum.Datum) {
				t.Push(d)
				t.Push(3.1)
			},
			expected: "3.1",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			m := makeMetrics()
			v := makeVM(test.i, m)
			d, err := m[test.d].GetDatum()
			testutil.FatalIfErr(t, err)
			test.setup(v.t, d)
			v.execute(v.t, v.prog[0])
			if v.terminate {
				t.Fatalf("Execution failed, see INFO log.")
			}
			d, err = m[test.d].GetDatum()
			testutil.FatalIfErr(t, err)
			if d.ValueString() != test.expected {
				t.Errorf("unexpected value for datum %#v, want: %s, got %s", d, test.expected, d.ValueString())
			}
		})
	}
}

func TestStrptimeWithTimezone(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	obj := &code.Object{Program: []code.Instr{{Opcode: code.Strptime, Operand: 0, SourceLine: 0}}}
	vm := New("strptimezone", obj, true, loc, false, false)
	vm.t = new(thread)
	vm.t.stack = make([]interface{}, 0)
	vm.t.Push("2012/01/18 06:25:00")
	vm.t.Push("2006/01/02 15:04:05")
	vm.execute(vm.t, obj.Program[0])
	if vm.t.time != time.Date(2012, 1, 18, 6, 25, 0, 0, loc) {
		t.Errorf("Time didn't parse with location: %s received", vm.t.time)
	}
}

func TestStrptimeWithoutTimezone(t *testing.T) {
	obj := &code.Object{Program: []code.Instr{{Opcode: code.Strptime, Operand: 0, SourceLine: 0}}}
	vm := New("strptimezone", obj, true, nil, false, false)
	vm.t = new(thread)
	vm.t.stack = make([]interface{}, 0)
	vm.t.Push("2012/01/18 06:25:00")
	vm.t.Push("2006/01/02 15:04:05")
	vm.execute(vm.t, obj.Program[0])
	if vm.t.time != time.Date(2012, 1, 18, 6, 25, 0, 0, time.UTC) {
		t.Errorf("Time didn't parse with location: %s received", vm.t.time)
	}
}

// code.Instructions with datum retrieve.
func TestDatumFetchInstrs(t *testing.T) {
	var m []*metrics.Metric
	m = append(m,
		metrics.NewMetric("a", "tst", metrics.Counter, metrics.Int),
		metrics.NewMetric("b", "tst", metrics.Counter, metrics.Float),
		metrics.NewMetric("c", "tst", metrics.Text, metrics.String))

	{
		// iget
		v := makeVM(code.Instr{Opcode: code.Iget}, m)
		d, err := m[0].GetDatum()
		testutil.FatalIfErr(t, err)
		datum.SetInt(d, 37, time.Now())
		v.t.Push(d)
		v.execute(v.t, v.prog[0])
		if v.terminate {
			t.Fatalf("Execution failed, see info log.")
		}
		i, err := v.t.PopInt()
		if err != nil {
			t.Fatalf("Execution failed, see info; %v", err)
		}
		if i != 37 {
			t.Errorf("unexpected value %d", i)
		}
	}

	{
		// fget
		v := makeVM(code.Instr{Opcode: code.Fget}, m)
		d, err := m[1].GetDatum()
		testutil.FatalIfErr(t, err)
		datum.SetFloat(d, 12.1, time.Now())
		v.t.Push(d)
		v.execute(v.t, v.prog[0])
		if v.terminate {
			t.Fatalf("Execution failed, see info log.")
		}
		i, err := v.t.PopFloat()
		if err != nil {
			t.Fatalf("Execution failed, see info: %v", err)
		}
		if i != 12.1 {
			t.Errorf("unexpected value %f", i)
		}
	}

	{
		// sget
		v := makeVM(code.Instr{Opcode: code.Sget}, m)
		d, err := m[2].GetDatum()
		testutil.FatalIfErr(t, err)
		datum.SetString(d, "aba", time.Now())
		v.t.Push(d)
		v.execute(v.t, v.prog[0])
		if v.terminate {
			t.Fatalf("Execution failed, see info log.")
		}
		i, err := v.t.PopString()
		if err != nil {
			t.Fatalf("Execution failed, see info log: %v", err)
		}
		if i != "aba" {
			t.Errorf("unexpected value %q", i)
		}
	}
}

func TestDeleteInstrs(t *testing.T) {
	var m []*metrics.Metric
	m = append(m,
		metrics.NewMetric("a", "tst", metrics.Counter, metrics.Int, "a"),
	)

	_, err := m[0].GetDatum("z")
	testutil.FatalIfErr(t, err)

	v := makeVM(code.Instr{Opcode: code.Expire, Operand: 1}, m)
	v.t.Push(time.Hour)
	v.t.Push("z")
	v.t.Push(m[0])
	v.execute(v.t, v.prog[0])
	if v.terminate {
		t.Fatal("execution failed, see info log")
	}
	lv := m[0].FindLabelValueOrNil([]string{"z"})
	if lv == nil {
		t.Fatalf("couldn;t find label value in metric %#v", m[0])
	}
	if lv.Expiry != time.Hour {
		t.Fatalf("Expiry not correct, is %v", lv.Expiry)
	}
}

func TestTimestampInstr(t *testing.T) {
	var m []*metrics.Metric
	now := time.Now().UTC()
	v := makeVM(code.Instr{Opcode: code.Timestamp}, m)
	v.execute(v.t, v.prog[0])
	if v.terminate {
		t.Fatal("execution failed, see info log")
	}
	tos := time.Unix(v.t.Pop().(int64), 0).UTC()
	if now.Before(tos) {
		t.Errorf("Expecting timestamp to be after %s, was %s", now, tos)
	}

	newT := time.Unix(37, 0).UTC()
	v.t.time = newT
	v.execute(v.t, v.prog[0])

	if v.terminate {
		t.Fatal("execution failed, see info log")
	}
	tos = time.Unix(v.t.Pop().(int64), 0).UTC()
	if tos != newT {
		t.Errorf("Expecting timestamp to be %s, was %s", newT, tos)
	}
}

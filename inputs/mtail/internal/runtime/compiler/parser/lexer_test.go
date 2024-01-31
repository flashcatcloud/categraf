// Copyright 2011 Google Inc. All Rights Reserved.
// This file is available under the Apache license.

package parser

import (
	"strings"
	"testing"

	"flashcat.cloud/categraf/inputs/mtail/internal/runtime/compiler/position"
	"flashcat.cloud/categraf/inputs/mtail/internal/testutil"
)

type lexerTest struct {
	name   string
	input  string
	tokens []Token
}

var lexerTests = []lexerTest{
	{name: "empty", tokens: []Token{
		{Kind: EOF, Pos: position.Position{Filename: "empty"}},
	}},
	{name: "spaces", input: " \t", tokens: []Token{
		{Kind: EOF, Pos: position.Position{Filename: "spaces", Startcol: 2, Endcol: 2}},
	}},
	{name: "newlines", input: "\n", tokens: []Token{
		{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "newlines", Line: 1, Endcol: -1}},
		{Kind: EOF, Pos: position.Position{Filename: "newlines", Line: 1}},
	}},
	{name: "comment", input: "# comment", tokens: []Token{
		{Kind: EOF, Pos: position.Position{Filename: "comment", Startcol: 9, Endcol: 9}},
	}},
	{name: "comment not at col 1", input: "  # comment", tokens: []Token{
		{Kind: EOF, Pos: position.Position{Filename: "comment not at col 1", Startcol: 11, Endcol: 11}},
	}},
	{name: "punctuation", input: "{}()[],", tokens: []Token{
		{Kind: LCURLY, Spelling: "{", Pos: position.Position{Filename: "punctuation"}},
		{Kind: RCURLY, Spelling: "}", Pos: position.Position{Filename: "punctuation", Startcol: 1, Endcol: 1}},
		{Kind: LPAREN, Spelling: "(", Pos: position.Position{Filename: "punctuation", Startcol: 2, Endcol: 2}},
		{Kind: RPAREN, Spelling: ")", Pos: position.Position{Filename: "punctuation", Startcol: 3, Endcol: 3}},
		{Kind: LSQUARE, Spelling: "[", Pos: position.Position{Filename: "punctuation", Startcol: 4, Endcol: 4}},
		{Kind: RSQUARE, Spelling: "]", Pos: position.Position{Filename: "punctuation", Startcol: 5, Endcol: 5}},
		{Kind: COMMA, Spelling: ",", Pos: position.Position{Filename: "punctuation", Startcol: 6, Endcol: 6}},
		{Kind: EOF, Pos: position.Position{Filename: "punctuation", Startcol: 7, Endcol: 7}},
	}},
	{name: "operators", input: "- + = ++ += < > <= >= == != * / << >> & | ^ ~ ** % || && =~ !~ --", tokens: []Token{
		{Kind: MINUS, Spelling: "-", Pos: position.Position{Filename: "operators"}},
		{Kind: PLUS, Spelling: "+", Pos: position.Position{Filename: "operators", Startcol: 2, Endcol: 2}},
		{Kind: ASSIGN, Spelling: "=", Pos: position.Position{Filename: "operators", Startcol: 4, Endcol: 4}},
		{Kind: INC, Spelling: "++", Pos: position.Position{Filename: "operators", Startcol: 6, Endcol: 7}},
		{Kind: ADD_ASSIGN, Spelling: "+=", Pos: position.Position{Filename: "operators", Startcol: 9, Endcol: 10}},
		{Kind: LT, Spelling: "<", Pos: position.Position{Filename: "operators", Startcol: 12, Endcol: 12}},
		{Kind: GT, Spelling: ">", Pos: position.Position{Filename: "operators", Startcol: 14, Endcol: 14}},
		{Kind: LE, Spelling: "<=", Pos: position.Position{Filename: "operators", Startcol: 16, Endcol: 17}},
		{Kind: GE, Spelling: ">=", Pos: position.Position{Filename: "operators", Startcol: 19, Endcol: 20}},
		{Kind: EQ, Spelling: "==", Pos: position.Position{Filename: "operators", Startcol: 22, Endcol: 23}},
		{Kind: NE, Spelling: "!=", Pos: position.Position{Filename: "operators", Startcol: 25, Endcol: 26}},
		{Kind: MUL, Spelling: "*", Pos: position.Position{Filename: "operators", Startcol: 28, Endcol: 28}},
		{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "operators", Startcol: 30, Endcol: 30}},
		{Kind: SHL, Spelling: "<<", Pos: position.Position{Filename: "operators", Startcol: 32, Endcol: 33}},
		{Kind: SHR, Spelling: ">>", Pos: position.Position{Filename: "operators", Startcol: 35, Endcol: 36}},
		{Kind: BITAND, Spelling: "&", Pos: position.Position{Filename: "operators", Startcol: 38, Endcol: 38}},
		{Kind: BITOR, Spelling: "|", Pos: position.Position{Filename: "operators", Startcol: 40, Endcol: 40}},
		{Kind: XOR, Spelling: "^", Pos: position.Position{Filename: "operators", Startcol: 42, Endcol: 42}},
		{Kind: NOT, Spelling: "~", Pos: position.Position{Filename: "operators", Startcol: 44, Endcol: 44}},
		{Kind: POW, Spelling: "**", Pos: position.Position{Filename: "operators", Startcol: 46, Endcol: 47}},
		{Kind: MOD, Spelling: "%", Pos: position.Position{Filename: "operators", Startcol: 49, Endcol: 49}},
		{Kind: OR, Spelling: "||", Pos: position.Position{Filename: "operators", Startcol: 51, Endcol: 52}},
		{Kind: AND, Spelling: "&&", Pos: position.Position{Filename: "operators", Startcol: 54, Endcol: 55}},
		{Kind: MATCH, Spelling: "=~", Pos: position.Position{Filename: "operators", Startcol: 57, Endcol: 58}},
		{Kind: NOT_MATCH, Spelling: "!~", Pos: position.Position{Filename: "operators", Startcol: 60, Endcol: 61}},
		{Kind: DEC, Spelling: "--", Pos: position.Position{Filename: "operators", Startcol: 63, Endcol: 64}},
		{Kind: EOF, Pos: position.Position{Filename: "operators", Startcol: 65, Endcol: 65}},
	}},
	{
		name:  "keywords",
		input: "counter\ngauge\nas\nby\nhidden\ndef\nnext\nconst\ntimer\notherwise\nelse\ndel\ntext\nafter\nstop\nhistogram\nbuckets\n",
		tokens: []Token{
			{Kind: COUNTER, Spelling: "counter", Pos: position.Position{Filename: "keywords", Endcol: 6}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 1, Startcol: 7, Endcol: -1}},
			{Kind: GAUGE, Spelling: "gauge", Pos: position.Position{Filename: "keywords", Line: 1, Endcol: 4}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 2, Startcol: 5, Endcol: -1}},
			{Kind: AS, Spelling: "as", Pos: position.Position{Filename: "keywords", Line: 2, Endcol: 1}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 3, Startcol: 2, Endcol: -1}},
			{Kind: BY, Spelling: "by", Pos: position.Position{Filename: "keywords", Line: 3, Endcol: 1}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 4, Startcol: 2, Endcol: -1}},
			{Kind: HIDDEN, Spelling: "hidden", Pos: position.Position{Filename: "keywords", Line: 4, Endcol: 5}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 5, Startcol: 6, Endcol: -1}},
			{Kind: DEF, Spelling: "def", Pos: position.Position{Filename: "keywords", Line: 5, Endcol: 2}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 6, Startcol: 3, Endcol: -1}},
			{Kind: NEXT, Spelling: "next", Pos: position.Position{Filename: "keywords", Line: 6, Endcol: 3}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 7, Startcol: 4, Endcol: -1}},
			{Kind: CONST, Spelling: "const", Pos: position.Position{Filename: "keywords", Line: 7, Endcol: 4}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 8, Startcol: 5, Endcol: -1}},
			{Kind: TIMER, Spelling: "timer", Pos: position.Position{Filename: "keywords", Line: 8, Endcol: 4}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 9, Startcol: 5, Endcol: -1}},
			{Kind: OTHERWISE, Spelling: "otherwise", Pos: position.Position{Filename: "keywords", Line: 9, Endcol: 8}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 10, Startcol: 9, Endcol: -1}},
			{Kind: ELSE, Spelling: "else", Pos: position.Position{Filename: "keywords", Line: 10, Endcol: 3}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 11, Startcol: 4, Endcol: -1}},
			{Kind: DEL, Spelling: "del", Pos: position.Position{Filename: "keywords", Line: 11, Endcol: 2}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 12, Startcol: 3, Endcol: -1}},
			{Kind: TEXT, Spelling: "text", Pos: position.Position{Filename: "keywords", Line: 12, Endcol: 3}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 13, Startcol: 4, Endcol: -1}},
			{Kind: AFTER, Spelling: "after", Pos: position.Position{Filename: "keywords", Line: 13, Endcol: 4}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 14, Startcol: 5, Endcol: -1}},
			{Kind: STOP, Spelling: "stop", Pos: position.Position{Filename: "keywords", Line: 14, Endcol: 3}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 15, Startcol: 4, Endcol: -1}},
			{Kind: HISTOGRAM, Spelling: "histogram", Pos: position.Position{Filename: "keywords", Line: 15, Endcol: 8}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 16, Startcol: 9, Endcol: -1}},
			{Kind: BUCKETS, Spelling: "buckets", Pos: position.Position{Filename: "keywords", Line: 16, Endcol: 6}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "keywords", Line: 17, Startcol: 7, Endcol: -1}},
			{Kind: EOF, Pos: position.Position{Filename: "keywords", Line: 17}},
		},
	},
	{
		name:  "builtins",
		input: "strptime\ntimestamp\ntolower\nlen\nstrtol\nsettime\ngetfilename\nint\nbool\nfloat\nstring\nsubst\n",
		tokens: []Token{
			{Kind: BUILTIN, Spelling: "strptime", Pos: position.Position{Filename: "builtins", Endcol: 7}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 1, Startcol: 8, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "timestamp", Pos: position.Position{Filename: "builtins", Line: 1, Endcol: 8}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 2, Startcol: 9, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "tolower", Pos: position.Position{Filename: "builtins", Line: 2, Endcol: 6}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 3, Startcol: 7, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "len", Pos: position.Position{Filename: "builtins", Line: 3, Endcol: 2}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 4, Startcol: 3, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "strtol", Pos: position.Position{Filename: "builtins", Line: 4, Endcol: 5}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 5, Startcol: 6, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "settime", Pos: position.Position{Filename: "builtins", Line: 5, Endcol: 6}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 6, Startcol: 7, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "getfilename", Pos: position.Position{Filename: "builtins", Line: 6, Endcol: 10}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 7, Startcol: 11, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "int", Pos: position.Position{Filename: "builtins", Line: 7, Endcol: 2}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 8, Startcol: 3, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "bool", Pos: position.Position{Filename: "builtins", Line: 8, Endcol: 3}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 9, Startcol: 4, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "float", Pos: position.Position{Filename: "builtins", Line: 9, Endcol: 4}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 10, Startcol: 5, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "string", Pos: position.Position{Filename: "builtins", Line: 10, Endcol: 5}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 11, Startcol: 6, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "subst", Pos: position.Position{Filename: "builtins", Line: 11, Endcol: 4}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "builtins", Line: 12, Startcol: 5, Endcol: -1}},
			{Kind: EOF, Pos: position.Position{Filename: "builtins", Line: 12}},
		},
	},
	{name: "numbers", input: "1 23 3.14 1.61.1 -1 -1.0 1h 0d 3d -1.5h 15m 24h0m0s 1e3 1e-3 .11 123.456e7", tokens: []Token{
		{Kind: INTLITERAL, Spelling: "1", Pos: position.Position{Filename: "numbers"}},
		{Kind: INTLITERAL, Spelling: "23", Pos: position.Position{Filename: "numbers", Startcol: 2, Endcol: 3}},
		{Kind: FLOATLITERAL, Spelling: "3.14", Pos: position.Position{Filename: "numbers", Startcol: 5, Endcol: 8}},
		{Kind: FLOATLITERAL, Spelling: "1.61", Pos: position.Position{Filename: "numbers", Startcol: 10, Endcol: 13}},
		{Kind: FLOATLITERAL, Spelling: ".1", Pos: position.Position{Filename: "numbers", Startcol: 14, Endcol: 15}},
		{Kind: INTLITERAL, Spelling: "-1", Pos: position.Position{Filename: "numbers", Startcol: 17, Endcol: 18}},
		{Kind: FLOATLITERAL, Spelling: "-1.0", Pos: position.Position{Filename: "numbers", Startcol: 20, Endcol: 23}},
		{Kind: DURATIONLITERAL, Spelling: "1h", Pos: position.Position{Filename: "numbers", Startcol: 25, Endcol: 26}},
		{Kind: DURATIONLITERAL, Spelling: "0d", Pos: position.Position{Filename: "numbers", Startcol: 28, Endcol: 29}},
		{Kind: DURATIONLITERAL, Spelling: "3d", Pos: position.Position{Filename: "numbers", Startcol: 31, Endcol: 32}},
		{Kind: DURATIONLITERAL, Spelling: "-1.5h", Pos: position.Position{Filename: "numbers", Startcol: 34, Endcol: 38}},
		{Kind: DURATIONLITERAL, Spelling: "15m", Pos: position.Position{Filename: "numbers", Startcol: 40, Endcol: 42}},
		{Kind: DURATIONLITERAL, Spelling: "24h0m0s", Pos: position.Position{Filename: "numbers", Startcol: 44, Endcol: 50}},
		{Kind: FLOATLITERAL, Spelling: "1e3", Pos: position.Position{Filename: "numbers", Startcol: 52, Endcol: 54}},
		{Kind: FLOATLITERAL, Spelling: "1e-3", Pos: position.Position{Filename: "numbers", Startcol: 56, Endcol: 59}},
		{Kind: FLOATLITERAL, Spelling: ".11", Pos: position.Position{Filename: "numbers", Startcol: 61, Endcol: 63}},
		{Kind: FLOATLITERAL, Spelling: "123.456e7", Pos: position.Position{Filename: "numbers", Startcol: 65, Endcol: 73}},
		{Kind: EOF, Pos: position.Position{Filename: "numbers", Startcol: 74, Endcol: 74}},
	}},
	{name: "identifier", input: "a be foo\nquux lines_total", tokens: []Token{
		{Kind: ID, Spelling: "a", Pos: position.Position{Filename: "identifier"}},
		{Kind: ID, Spelling: "be", Pos: position.Position{Filename: "identifier", Startcol: 2, Endcol: 3}},
		{Kind: ID, Spelling: "foo", Pos: position.Position{Filename: "identifier", Startcol: 5, Endcol: 7}},
		{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "identifier", Line: 1, Startcol: 8, Endcol: -1}},
		{Kind: ID, Spelling: "quux", Pos: position.Position{Filename: "identifier", Line: 1, Endcol: 3}},
		{Kind: ID, Spelling: "lines_total", Pos: position.Position{Filename: "identifier", Line: 1, Startcol: 5, Endcol: 15}},
		{Kind: EOF, Pos: position.Position{Filename: "identifier", Line: 1, Startcol: 16, Endcol: 16}},
	}},
	{name: "regex", input: "/asdf/", tokens: []Token{
		{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "regex"}},
		{Kind: REGEX, Spelling: "asdf", Pos: position.Position{Filename: "regex", Startcol: 1, Endcol: 4}},
		{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "regex", Startcol: 5, Endcol: 5}},
		{Kind: EOF, Pos: position.Position{Filename: "regex", Startcol: 6, Endcol: 6}},
	}},
	{name: "regex with escape", input: `/asdf\//`, tokens: []Token{
		{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "regex with escape"}},
		{Kind: REGEX, Spelling: `asdf/`, Pos: position.Position{Filename: "regex with escape", Startcol: 1, Endcol: 6}},
		{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "regex with escape", Startcol: 7, Endcol: 7}},
		{Kind: EOF, Pos: position.Position{Filename: "regex with escape", Startcol: 8, Endcol: 8}},
	}},
	{name: "regex with escape and special char", input: `/foo\d\//`, tokens: []Token{
		{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "regex with escape and special char"}},
		{Kind: REGEX, Spelling: `foo\d/`, Pos: position.Position{Filename: "regex with escape and special char", Startcol: 1, Endcol: 7}},
		{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "regex with escape and special char", Startcol: 8, Endcol: 8}},
		{Kind: EOF, Pos: position.Position{Filename: "regex with escape and special char", Startcol: 9, Endcol: 9}},
	}},
	{name: "capref", input: "$foo $1", tokens: []Token{
		{Kind: CAPREF_NAMED, Spelling: "foo", Pos: position.Position{Filename: "capref", Endcol: 3}},
		{Kind: CAPREF, Spelling: "1", Pos: position.Position{Filename: "capref", Startcol: 5, Endcol: 6}},
		{Kind: EOF, Pos: position.Position{Filename: "capref", Startcol: 7, Endcol: 7}},
	}},
	{name: "numerical capref", input: "$1", tokens: []Token{
		{Kind: CAPREF, Spelling: "1", Pos: position.Position{Filename: "numerical capref", Endcol: 1}},
		{Kind: EOF, Pos: position.Position{Filename: "numerical capref", Startcol: 2, Endcol: 2}},
	}},
	{name: "capref with trailing punc", input: "$foo,", tokens: []Token{
		{Kind: CAPREF_NAMED, Spelling: "foo", Pos: position.Position{Filename: "capref with trailing punc", Endcol: 3}},
		{Kind: COMMA, Spelling: ",", Pos: position.Position{Filename: "capref with trailing punc", Startcol: 4, Endcol: 4}},
		{Kind: EOF, Pos: position.Position{Filename: "capref with trailing punc", Startcol: 5, Endcol: 5}},
	}},
	{name: "quoted string", input: `"asdf"`, tokens: []Token{
		{Kind: STRING, Spelling: `asdf`, Pos: position.Position{Filename: "quoted string", Endcol: 5}},
		{Kind: EOF, Pos: position.Position{Filename: "quoted string", Startcol: 6, Endcol: 6}},
	}},
	{name: "escaped quote in quoted string", input: `"\""`, tokens: []Token{
		{Kind: STRING, Spelling: `"`, Pos: position.Position{Filename: "escaped quote in quoted string", Endcol: 3}},
		{Kind: EOF, Pos: position.Position{Filename: "escaped quote in quoted string", Startcol: 4, Endcol: 4}},
	}},
	{name: "decorator", input: `@foo`, tokens: []Token{
		{Kind: DECO, Spelling: "foo", Pos: position.Position{Filename: "decorator", Endcol: 3}},
		{Kind: EOF, Pos: position.Position{Filename: "decorator", Startcol: 4, Endcol: 4}},
	}},
	{
		name: "large program",
		input: "/(?P<date>[[:digit:]-\\/ ])/ {\n" +
			"  strptime($date, \"%Y/%m/%d %H:%M:%S\")\n" +
			"  foo++\n" +
			"}",
		tokens: []Token{
			{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "large program"}},
			{Kind: REGEX, Spelling: "(?P<date>[[:digit:]-/ ])", Pos: position.Position{Filename: "large program", Startcol: 1, Endcol: 25}},
			{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "large program", Startcol: 26, Endcol: 26}},
			{Kind: LCURLY, Spelling: "{", Pos: position.Position{Filename: "large program", Startcol: 28, Endcol: 28}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "large program", Line: 1, Startcol: 29, Endcol: -1}},
			{Kind: BUILTIN, Spelling: "strptime", Pos: position.Position{Filename: "large program", Line: 1, Startcol: 2, Endcol: 9}},
			{Kind: LPAREN, Spelling: "(", Pos: position.Position{Filename: "large program", Line: 1, Startcol: 10, Endcol: 10}},
			{Kind: CAPREF_NAMED, Spelling: "date", Pos: position.Position{Filename: "large program", Line: 1, Startcol: 11, Endcol: 15}},
			{Kind: COMMA, Spelling: ",", Pos: position.Position{Filename: "large program", Line: 1, Startcol: 16, Endcol: 16}},
			{Kind: STRING, Spelling: "%Y/%m/%d %H:%M:%S", Pos: position.Position{Filename: "large program", Line: 1, Startcol: 18, Endcol: 36}},
			{Kind: RPAREN, Spelling: ")", Pos: position.Position{Filename: "large program", Line: 1, Startcol: 37, Endcol: 37}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "large program", Line: 2, Startcol: 38, Endcol: -1}},
			{Kind: ID, Spelling: "foo", Pos: position.Position{Filename: "large program", Line: 2, Startcol: 2, Endcol: 4}},
			{Kind: INC, Spelling: "++", Pos: position.Position{Filename: "large program", Line: 2, Startcol: 5, Endcol: 6}},
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "large program", Line: 3, Startcol: 7, Endcol: -1}},
			{Kind: RCURLY, Spelling: "}", Pos: position.Position{Filename: "large program", Line: 3}},
			{Kind: EOF, Pos: position.Position{Filename: "large program", Line: 3, Startcol: 1, Endcol: 1}},
		},
	},
	{
		name: "linecount",
		input: "# comment\n" +
			"# blank line\n" +
			"\n" +
			"foo",
		tokens: []Token{
			{Kind: NL, Spelling: "\n", Pos: position.Position{Filename: "linecount", Line: 3, Startcol: 12, Endcol: -1}},
			{Kind: ID, Spelling: "foo", Pos: position.Position{Filename: "linecount", Line: 3, Endcol: 2}},
			{Kind: EOF, Pos: position.Position{Filename: "linecount", Line: 3, Startcol: 3, Endcol: 3}},
		},
	},
	// errors
	{name: "unexpected char", input: "?", tokens: []Token{
		{Kind: INVALID, Spelling: "Unexpected input: '?'", Pos: position.Position{Filename: "unexpected char"}},
		{Kind: EOF, Pos: position.Position{Filename: "unexpected char", Startcol: 1, Endcol: 1}},
	}},
	{name: "unterminated regex", input: "/foo\n", tokens: []Token{
		{Kind: DIV, Spelling: "/", Pos: position.Position{Filename: "unterminated regex"}},
		{Kind: INVALID, Spelling: "Unterminated regular expression: \"/foo\"", Pos: position.Position{Filename: "unterminated regex", Startcol: 1, Endcol: 3}},
		{Kind: EOF, Pos: position.Position{Filename: "unterminated regex", Startcol: 4, Endcol: 4}},
	}},
	{name: "unterminated quoted string", input: "\"foo\n", tokens: []Token{
		{Kind: INVALID, Spelling: "Unterminated quoted string: \"\\\"foo\"", Pos: position.Position{Filename: "unterminated quoted string", Endcol: 3}},
		{Kind: EOF, Pos: position.Position{Filename: "unterminated quoted string", Startcol: 4, Endcol: 4}},
	}},
}

// collect gathers the emitted items into a slice.
func collect(t *lexerTest) (tokens []Token) {
	// Hack to count divs seen for regex tests.
	inRegexSet := false
	l := NewLexer(t.name, strings.NewReader(t.input))
	for {
		tok := l.NextToken()
		// Hack to simulate context signal from parser.
		if tok.Kind == DIV && (strings.Contains(t.name, "regex") || strings.HasPrefix(t.name, "large program")) && !inRegexSet {
			l.InRegex = true
			inRegexSet = true
		}
		tokens = append(tokens, tok)
		if tok.Kind == EOF {
			return
		}
	}
}

func TestLex(t *testing.T) {
	for _, tc := range lexerTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tokens := collect(&tc)

			testutil.ExpectNoDiff(t, tc.tokens, tokens, testutil.AllowUnexported(Token{}, position.Position{}))
		})
	}
}

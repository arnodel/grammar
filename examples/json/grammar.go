package json

import (
	"github.com/arnodel/grammar"
)

//go:generate genparse

type Token = grammar.SimpleToken

// Json ::= Number | String | Null | Bool | Array | Dict
type Json struct {
	grammar.OneOf // This tells the parser exactly one of the fields below should be populated
	*Number
	*String
	*Null
	*Bool
	*Array
	*Dict
}

// Number ::= <number token>
type Number struct {
	grammar.Seq
	Value Token `tok:"number"` // This tells the parser that a token of type "number" should be used
}

// String ::= <string token>
type String struct {
	grammar.Seq
	Value Token `tok:"string"`
}

// Null ::= "null"
type Null struct {
	grammar.Seq
	Value Token `tok:"null,null"`
}

// Bool ::= <bool token>
type Bool struct {
	grammar.Seq
	Value Token `tok:"bool"`
}

// Array ::= "[" [ArrayBody] "]"
type Array struct {
	grammar.Seq
	Open  grammar.Match `tok:"op,["` // This tells the parser a token of type "op" with value "[" should be used
	Items []Json        `sep:"op,,"` // This tells the parse items should be separater by a token of type "op" with value ","
	Close grammar.Match `tok:"op,]"`
}

// Dict ::= "{" [DictBody] "}"
type Dict struct {
	grammar.Seq
	Open  grammar.Match `tok:"op,{"`
	Items []DictItem    `sep:"op,,"`
	Close grammar.Match `tok:"op,}"`
}

type DictItem struct {
	grammar.Seq
	Key   String
	Colon grammar.Match `tok:"op,:"`
	Value Json
}

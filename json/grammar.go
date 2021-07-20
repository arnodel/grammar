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
	Open       Token `tok:"op,["` // This tells the parser a token of type "op" with value "[" should be used
	*ArrayBody       // A pointer field is optional
	Close      Token `tok:"op,]"`
}

// ArrayBody ::= Json ("," Json)*
type ArrayBody struct {
	grammar.Seq
	First Json // A non-pointer field is compulsory
	Items []ArrayItem
}

type ArrayItem struct {
	grammar.Seq
	Comma Token `tok:"op,,"`
	Value Json
}

// Dict ::= "{" [DictBody] "}"
type Dict struct {
	grammar.Seq
	Open Token `tok:"op,{"`
	*DictBody
	Close Token `tok:"op,}"`
}

// DictBody ::= String ":" Json ("," String : Json)*
type DictBody struct {
	grammar.Seq
	First KeyValue
	Items []DictItem
}

type KeyValue struct {
	grammar.Seq
	Key   String
	Colon Token `tok:"op,:"`
	Value Json
}

type DictItem struct {
	grammar.Seq
	Comma Token `tok:"op,,"`
	KeyValue
}

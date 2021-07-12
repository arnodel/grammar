package json

import (
	"github.com/arnodel/grammar"
)

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
	Value Token `tok:"number"` // This tells the parser that a token of type "number" should be used
}

// String ::= <string token>
type String struct {
	Value Token `tok:"string"`
}

// Null ::= "null"
type Null struct {
	Value Token `tok:"null,null"`
}

// Bool ::= <bool token>
type Bool struct {
	Value Token `tok:"bool"`
}

// Array ::= "[" [ArrayBody] "]"
type Array struct {
	Open       Token `tok:"op,["` // This tells the parser a token of type "op" with value "[" should be used
	*ArrayBody       // A pointer field is optional
	Close      Token `tok:"op,]"`
}

// ArrayBody ::= Json ("," Json)*
type ArrayBody struct {
	First Json // A non-pointer field is compulsory
	Items []ArrayItem
}

type ArrayItem struct {
	Comma Token `tok:"op,,"`
	Value Json
}

// Dict ::= "{" [DictBody] "}"
type Dict struct {
	Open Token `tok:"op,{"`
	*DictBody
	Close Token `tok:"op,}"`
}

// DictBody ::= String ":" Json ("," String : Json)*
type DictBody struct {
	First KeyValue
	Items []DictItem
}

type KeyValue struct {
	Key   String
	Colon Token `tok:"op,:"`
	Value Json
}

type DictItem struct {
	Comma Token `tok:"op,,"`
	KeyValue
}

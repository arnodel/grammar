package grammar

import (
	"testing"
)

// Rule types whose Parse method has a pointer receiver (Tok-based rules and
// nested OneOfN) must be accepted as type arguments of every OneOfN.
type oneOfNum struct {
	Tok `tok:"number"`
}

type oneOfStr struct {
	Tok `tok:"string"`
}

type oneOfBool struct {
	Tok `tok:"bool"`
}

type oneOfPair struct {
	Seq
	Lhs oneOfNum
	Rhs OneOf2[oneOfNum, oneOfStr]
}

type oneOfValue struct {
	OneOf3[oneOfBool, oneOfPair, OneOf2[oneOfNum, oneOfStr]]
}

// Every arity must accept the same kinds of type argument.
var (
	_ Parser = &OneOf4[oneOfNum, oneOfStr, oneOfBool, oneOfPair]{}
	_ Parser = &OneOf5[oneOfNum, oneOfStr, oneOfBool, oneOfPair, oneOfValue]{}
	_ Parser = &OneOf6[oneOfNum, oneOfStr, oneOfBool, oneOfPair, oneOfValue, OneOf2[oneOfNum, oneOfStr]]{}
)

func TestGenericOneOf6LastAlternative(t *testing.T) {
	type last struct {
		OneOf6[oneOfNum, oneOfNum, oneOfNum, oneOfNum, oneOfNum, oneOfStr]
	}
	var v last
	err := Parse(&v, NewSimpleTokenStream([]Token{SimpleToken{TokType: "string", TokValue: `"z"`}}))
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := v.Value.(*oneOfStr); !ok || s.TokValue != `"z"` {
		t.Errorf("got %#v", v.Value)
	}
}

func TestGenericOneOfWithPointerReceiverRules(t *testing.T) {
	toks := []Token{
		SimpleToken{TokType: "number", TokValue: "1"},
		SimpleToken{TokType: "string", TokValue: `"a"`},
	}
	var v oneOfValue
	if err := Parse(&v, NewSimpleTokenStream(toks)); err != nil {
		t.Fatal(err)
	}
	pair, ok := v.Value.(*oneOfPair)
	if !ok {
		t.Fatalf("expected *oneOfPair, got %T", v.Value)
	}
	if pair.Lhs.TokValue != "1" {
		t.Errorf("Lhs: got %q", pair.Lhs.TokValue)
	}
	if s, ok := pair.Rhs.Value.(*oneOfStr); !ok || s.TokValue != `"a"` {
		t.Errorf("Rhs: got %#v", pair.Rhs.Value)
	}

	var w oneOfValue
	err := Parse(&w, NewSimpleTokenStream([]Token{SimpleToken{TokType: "op", TokValue: "+"}}))
	if err == nil {
		t.Fatal("expected an error")
	}
	if w.Value != nil {
		t.Errorf("expected nil Value on failure, got %T", w.Value)
	}
}

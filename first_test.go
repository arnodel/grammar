package grammar

import (
	"bytes"
	"io"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// A grammar exercising everything first sets have to deal with: OneOf with
// pointer and repeated alternatives, generic OneOfN, left recursion, optional
// and repeated fields, drop, separators and trailing separators, lookahead,
// Empty, token types with and without tags, and a custom rule.

var fsTokenise = SimpleTokeniser([]TokenDef{
	{Ptn: `[ \t]+`},
	{Name: "nl", Ptn: `\n`},
	{Name: "kw", Ptn: `let|print`},
	{Name: "id", Ptn: `[a-z]+`},
	{Name: "num", Ptn: `[0-9]+`},
	{Name: "op", Ptn: `[-+*(),;={}]`},
})

type fsProg struct {
	Seq   `drop:"nl"`
	Stmts []fsStmt `sep:"op,;" trail:"optional"`
}

type fsStmt struct {
	OneOf
	Let   *fsLet
	Print *fsPrint
	Guard *fsGuard
	Block *fsBlock
	Expr  *fsExpr
}

type fsLet struct {
	Seq
	Kw   Match       `tok:"kw,let"`
	Name SimpleToken `tok:"id"`
	Init *fsInit
}

type fsInit struct {
	Seq
	Eq Match `tok:"op,="`
	E  fsExpr
}

// A custom rule after a keyword.
type fsPrint struct {
	Seq
	Kw   Match `tok:"kw,print"`
	Word fsCustom
}

type fsCustom struct{ Text string }

func (c *fsCustom) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	tok, err := TokenOptions{TokenParseOptions: []TokenParseOptions{{TokenType: "id"}}}.MatchNextToken(s)
	if err != nil {
		return err
	}
	c.Text = tok.Value()
	return nil
}

func (c *fsCustom) PrettyWrite(out io.Writer, r any, pfx string, indent int) error {
	return writeLine(out, pfx, indent, "custom "+c.Text)
}

// A lookahead: only a call may follow.
type fsGuard struct {
	Seq
	Look Match `tok:"id*"`
	Call fsCall
}

type fsBlock struct {
	Seq
	Open  Match `tok:"op,{"`
	Body  fsProg
	Close Match `tok:"op,}"`
}

// Expr ::= Expr "+" Term | Term
type fsExpr struct {
	OneOf
	Sum  *fsSum
	Term *fsTerm
}

type fsSum struct {
	Seq
	Lhs fsExpr
	Op  Match `tok:"op,+"`
	Rhs fsTerm
}

type fsTerm struct {
	OneOf2[fsProduct, fsAtom]
}

type fsProduct struct {
	Seq
	Factors []fsAtom `sep:"op,*" size:"2-"`
}

type fsAtom struct {
	OneOf
	Num   *fsNum
	Id    *SimpleToken `tok:"id"`
	Paren *fsParen
	Neg   *fsNeg
	Empty *fsEmptyAtom
}

type fsNum struct {
	Tok `tok:"num"`
}

type fsParen struct {
	Seq
	Open  Match `tok:"op,("`
	E     fsExpr
	Close Match `tok:"op,)"`
}

type fsNeg struct {
	Seq
	Minus Match `tok:"op,-"`
	E     fsAtom
}

// Matches nothing, after a comma.
type fsEmptyAtom struct {
	Seq
	Comma Match `tok:"op,,"`
	E     Empty
}

type fsCall struct {
	Seq
	Fn    SimpleToken `tok:"id"`
	Open  Match       `tok:"op,("`
	Args  []fsExpr    `sep:"op,,"`
	Close Match       `tok:"op,)"`
}

// Only left recursive: can match nothing.
type fsLoop struct {
	OneOf
	Again *fsLoopStep
}

type fsLoopStep struct {
	Seq
	L fsLoop
	X Match `tok:"id"`
}

func optStrings(opts []TokenParseOptions) string {
	strs := make([]string, len(opts))
	for i, o := range opts {
		strs[i] = o.String()
		if o.DoNotConsume {
			strs[i] += "*"
		}
	}
	sort.Strings(strs)
	return strings.Join(strs, " | ")
}

func TestFirstSets(t *testing.T) {
	tests := []struct {
		typ      reflect.Type
		any      bool
		nullable bool
		opts     string
		drop     string
	}{
		{reflect.TypeFor[fsNum](), false, false, "type num", ""},
		{reflect.TypeFor[fsAtom](), false, false, `type id | type num | type op,value "(" | type op,value "," | type op,value "-"`, ""},
		{reflect.TypeFor[fsTerm](), false, false, `type id | type num | type op,value "(" | type op,value "," | type op,value "-"`, ""},
		{reflect.TypeFor[fsExpr](), false, false, `type id | type num | type op,value "(" | type op,value "," | type op,value "-"`, ""},
		{reflect.TypeFor[fsLet](), false, false, `type kw,value "let"`, ""},
		{reflect.TypeFor[fsGuard](), false, false, `type id*`, ""},
		{reflect.TypeFor[fsStmt](), false, false, `type id | type id* | type kw,value "let" | type kw,value "print" | type num | type op,value "(" | type op,value "," | type op,value "-" | type op,value "{"`, ""},
		{reflect.TypeFor[fsProg](), false, true, `type id | type id* | type kw,value "let" | type kw,value "print" | type num | type op,value "(" | type op,value "," | type op,value "-" | type op,value "{"`, "type nl"},
		{reflect.TypeFor[fsCustom](), true, false, "", ""},
		{reflect.TypeFor[fsEmptyAtom](), false, false, `type op,value ","`, ""},
		{reflect.TypeFor[Empty](), false, true, "", ""},
		{reflect.TypeFor[fsLoop](), true, false, "", ""},
	}
	for _, tt := range tests {
		f := firstOfType(tt.typ)
		if f.any != tt.any || f.nullable != tt.nullable || optStrings(f.opts) != tt.opts || optStrings(f.drop) != tt.drop {
			t.Errorf("%s: any=%v nullable=%v opts=%q drop=%q", tt.typ, f.any, f.nullable, optStrings(f.opts), optStrings(f.drop))
		}
	}
	// A field of a token type with a tag.
	f := FirstSetOf[SimpleToken](TokenOptions{TokenParseOptions: []TokenParseOptions{{TokenType: "id"}}})
	if f.any || f.nullable || optStrings(f.opts) != "type id" {
		t.Errorf("SimpleToken field: %+v", f)
	}
	if f := FirstSetOf[Match](TokenOptions{}); !f.nullable {
		t.Errorf("Match without a tag: %+v", f)
	}
	tok := SimpleToken{TokType: "num", TokValue: "1"}
	if !firstOfType(reflect.TypeFor[fsNum]()).Accepts(tok) || firstOfType(reflect.TypeFor[fsLet]()).Accepts(tok) {
		t.Error("Accepts")
	}
}

// parseOutcome summarises a parse so that two can be compared: the pretty
// output on success, the error details on failure.
func parseOutcome(src string) string {
	toks, err := fsTokenise(src)
	if err != nil {
		return "tokeniser: " + err.Error()
	}
	var prog fsProg
	if perr := Parse(&prog, toks, Exhaust); perr != nil {
		s := perr.Position().String() + " " + perr.Type() + " " + perr.Value() + " [" + optStrings(perr.TokenParseOptions) + "]"
		if perr.Err != nil {
			s += " " + perr.Err.Error()
		}
		return s
	}
	var buf bytes.Buffer
	PrettyWrite(&buf, &prog)
	return buf.String()
}

// TestFirstSetsDifferential parses random token sequences with and without
// skipping, which must not change the outcome, including the error.
func TestFirstSetsDifferential(t *testing.T) {
	vocab := []string{"0", "1", "x", "f", "let", "print", "+", "-", "*", "(", ")", ",", ";", "=", "{", "}", "\n"}
	rng := rand.New(rand.NewSource(1))
	defer func() { firstSetsEnabled = true }()
	successes := 0
	for i := 0; i < 20000; i++ {
		n := rng.Intn(10)
		parts := make([]string, n)
		for j := range parts {
			parts[j] = vocab[rng.Intn(len(vocab))]
		}
		src := strings.Join(parts, " ")
		firstSetsEnabled = true
		with := parseOutcome(src)
		firstSetsEnabled = false
		without := parseOutcome(src)
		if with != without {
			t.Fatalf("%q:\nwith first sets:\n%s\nwithout:\n%s", src, with, without)
		}
		if !strings.HasPrefix(with, "Ln ") {
			successes++
		}
	}
	if successes < 100 {
		t.Errorf("only %d successful parses, the inputs are not varied enough", successes)
	}
}

func TestFirstSetsDifferentialExamples(t *testing.T) {
	// Hand-picked inputs covering the constructs.
	for _, src := range []string{
		"", "let x", "let x = 1 + 2 * -3", "f(1, x, (2))", "{ let y = f() ; print z }", "x ; y ;", "1 + ,", "f(1\n, 2)",
		"print 1", "print", "let", "x(", "(1 + 2) * 3 + f(4)", "{\n}\n", "1 * 2 * 3 + 4 * 5", ", + ,",
	} {
		firstSetsEnabled = true
		with := parseOutcome(src)
		firstSetsEnabled = false
		without := parseOutcome(src)
		firstSetsEnabled = true
		if with != without {
			t.Errorf("%q:\nwith first sets:\n%s\nwithout:\n%s", src, with, without)
		}
	}
}

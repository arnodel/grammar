package sql

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/arnodel/grammar"
)

// Randomised tests of the parser on this grammar, in two parts.
//
// Expressions are checked against an oracle: an independent operator
// precedence (Pratt) parser for the operator subset of the expression
// grammar, written from the precedence table in expr.go.  Random
// expressions, well formed or not, must parse to the same tree with both or
// fail with both.  This pins the precedence machinery (levels, bounds,
// climbing, groups) on a real grammar, independently of the examples in
// TestExprPrecedence.
//
// Statements are checked for robustness: random token soups and mutations of
// the corpus must never make the parser panic, and when a statement parses,
// the tokens stored in the tree must be those of the input, in order.
//
// Both are Go fuzz targets (go test -fuzz=FuzzExpr, -fuzz=FuzzStmt), and
// TestExprOracle and TestStmtRandom run them on a fixed random sample so
// that go test exercises them too.  The generators make their choices from
// the fuzz input bytes, so the fuzzer can steer them.

// A chooser makes the random choices of the generators from a byte string.
// Past the end of the bytes it returns zeros, which end the generation.
type chooser struct {
	data []byte
	i    int
}

func (c *chooser) next() int {
	if c.i >= len(c.data) {
		c.i++
		return 0
	}
	b := c.data[c.i]
	c.i++
	return int(b)
}

func (c *chooser) pick(n int) int { return c.next() % n }

func (c *chooser) done() bool { return c.i >= len(c.data) }

// ---- Expressions ----

var (
	genAtoms   = []string{"a", "b", "t.c", "u.v.w", "1", "2.5", "'s'", "?", "NULL", "TRUE", "FIRST", `"q"`}
	genInfix   = []string{"OR", "AND", "=", "==", "!=", "<>", "<", ">", "<=", ">=", "&", "|", "<<", ">>", "+", "-", "*", "/", "%", "||", "->", "->>"}
	genPrefix  = []string{"NOT", "-", "+", "~"}
	genPattern = []string{"LIKE", "GLOB", "REGEXP", "MATCH"}
	genNoise   = []string{")", "(", ",", ".", "END", ";", "IS", "BETWEEN", "AND", "NOT", "NULL", "COLLATE", "IN", "ESCAPE", "DISTINCT", "FROM", "*", "f"}
	genVocab   = concat(genAtoms, genInfix, genPrefix, genPattern, genNoise)
)

func concat(xs ...[]string) (res []string) {
	for _, x := range xs {
		res = append(res, x...)
	}
	return res
}

// genExpr appends the tokens of a random expression, mostly well formed, to
// out.
func genExpr(c *chooser, depth int, out *[]string) {
	emit := func(toks ...string) { *out = append(*out, toks...) }
	sub := func() { genExpr(c, depth-1, out) }
	if depth == 0 || c.done() {
		emit(genAtoms[c.pick(len(genAtoms))])
		return
	}
	switch c.pick(16) {
	case 0, 1, 2, 3:
		emit(genAtoms[c.pick(len(genAtoms))])
	case 4, 5, 6, 7:
		sub()
		emit(genInfix[c.pick(len(genInfix))])
		sub()
	case 8:
		emit(genPrefix[c.pick(len(genPrefix))])
		sub()
	case 9:
		emit("(")
		sub()
		emit(")")
	case 10:
		emit("(")
		sub()
		emit(",")
		sub()
		emit(")")
	case 11:
		emit("f", "(")
		if c.pick(4) == 0 {
			emit("*")
		}
		if c.pick(4) == 0 {
			emit("DISTINCT")
		}
		for n := c.pick(3); n > 0; n-- {
			sub()
			if n > 1 {
				emit(",")
			}
		}
		emit(")")
	case 12:
		sub()
		switch c.pick(6) {
		case 0:
			emit("COLLATE", "nocase")
		case 1:
			emit("ISNULL")
		case 2:
			emit("NOTNULL")
		case 3:
			emit("NOT", "NULL")
		case 4:
			emit("IS")
			if c.pick(2) == 0 {
				emit("NOT")
			}
			if c.pick(2) == 0 {
				emit("DISTINCT", "FROM")
			}
			sub()
		case 5:
			if c.pick(2) == 0 {
				emit("NOT")
			}
			emit("BETWEEN")
			sub()
			emit("AND")
			sub()
		}
	case 13:
		sub()
		if c.pick(2) == 0 {
			emit("NOT")
		}
		if c.pick(2) == 0 {
			emit(genPattern[c.pick(len(genPattern))])
			sub()
			if c.pick(2) == 0 {
				emit("ESCAPE")
				sub()
			}
		} else {
			emit("IN")
			switch c.pick(3) {
			case 0:
				emit("(")
				for n := c.pick(3); n > 0; n-- {
					sub()
					if n > 1 {
						emit(",")
					}
				}
				emit(")")
			case 1:
				emit("t")
			case 2:
				emit("t.u")
			}
		}
	case 14:
		sub()
		emit(genNoise[c.pick(len(genNoise))])
	case 15:
		emit(genNoise[c.pick(len(genNoise))])
		sub()
	}
}

// The oracle: a Pratt parser over the tokens of an expression, producing
// the same S-expressions as lisp.  It fails by panicking with errNoParse.

var errNoParse = errors.New("no parse")

type oracle struct {
	toks []grammar.SimpleToken
	i    int
}

func (o *oracle) peek() grammar.SimpleToken {
	if o.i < len(o.toks) {
		return o.toks[o.i]
	}
	return grammar.SimpleToken{TokType: grammar.EOF.TokType}
}

func (o *oracle) advance() grammar.SimpleToken {
	tok := o.peek()
	o.i++
	return tok
}

func (o *oracle) is(typ, val string) bool {
	tok := o.peek()
	return tok.TokType == typ && tok.TokValue == val
}

func (o *oracle) accept(typ, val string) bool {
	if o.is(typ, val) {
		o.i++
		return true
	}
	return false
}

func (o *oracle) expect(typ, val string) {
	if !o.accept(typ, val) {
		panic(errNoParse)
	}
}

type oNode interface{ String() string }

type oAtom string

func (a oAtom) String() string { return string(a) }

// oParen is a parenthesised expression, transparent in the output but
// stopping OR and AND from being flattened through it.
type oParen struct{ x oNode }

func (p oParen) String() string { return p.x.String() }

type oForm struct {
	head string
	args []oNode
}

func (f oForm) String() string {
	parts := []string{f.head}
	for _, a := range f.args {
		parts = append(parts, a.String())
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func oracleParse(toks []grammar.SimpleToken) (res string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if r != errNoParse {
				panic(r)
			}
			res, ok = "", false
		}
	}()
	o := &oracle{toks: toks}
	n := o.expr(0)
	if o.peek().TokType != grammar.EOF.TokType {
		return "", false
	}
	return n.String(), true
}

// The levels of the binary operators, from the table in expr.go.
var oInfix = map[string]int{
	"OR": 1, "AND": 2,
	"=": 4, "==": 4, "!=": 4, "<>": 4,
	"<": 5, ">": 5, "<=": 5, ">=": 5,
	"&": 6, "|": 6, "<<": 6, ">>": 6,
	"+": 7, "-": 7,
	"*": 8, "/": 8, "%": 8,
	"||": 9, "->": 9, "->>": 9,
}

func isOpTok(tok grammar.SimpleToken) bool {
	switch tok.TokType {
	case "op", "kw", "bitor", "concat":
		return true
	}
	return false
}

// expr parses an expression under a precedence bound: an infix or postfix
// operator applies only if its level is above the bound, and its right
// operands are parsed under its level.
func (o *oracle) expr(bound int) oNode {
	left := o.primary()
	for {
		tok := o.peek()
		if lvl, ok := oInfix[tok.TokValue]; ok && isOpTok(tok) {
			if lvl <= bound {
				return left
			}
			o.advance()
			right := o.expr(lvl)
			if tok.TokValue == "OR" || tok.TokValue == "AND" {
				// n-ary, as in the grammar, where the operator rule takes all
				// the terms at its level.
				args := []oNode{left, right}
				for o.accept("kw", tok.TokValue) {
					args = append(args, o.expr(lvl))
				}
				left = oForm{tok.TokValue, args}
			} else {
				left = oForm{tok.TokValue, []oNode{left, right}}
			}
			continue
		}
		if tok.TokType != "kw" {
			return left
		}
		switch tok.TokValue {
		case "COLLATE":
			if 11 <= bound {
				return left
			}
			o.advance()
			left = oForm{"COLLATE", []oNode{left, oAtom(o.name())}}
		case "ISNULL", "NOTNULL":
			if 4 <= bound {
				return left
			}
			o.advance()
			left = oForm{tok.TokValue, []oNode{left}}
		case "IS":
			if 4 <= bound {
				return left
			}
			o.advance()
			head := "IS"
			if o.accept("kw", "NOT") {
				head += " NOT"
			}
			if o.accept("kw", "DISTINCT") {
				o.expect("kw", "FROM")
				head += " DISTINCT FROM"
			}
			left = oForm{head, []oNode{left, o.expr(4)}}
		case "NOT", "BETWEEN", "IN", "LIKE", "GLOB", "REGEXP", "MATCH":
			if 4 <= bound {
				return left
			}
			o.advance()
			not := ""
			if tok.TokValue == "NOT" {
				if o.accept("kw", "NULL") {
					left = oForm{"NOTNULL", []oNode{left}}
					continue
				}
				not = "NOT "
				tok = o.advance()
				if tok.TokType != "kw" {
					panic(errNoParse)
				}
			}
			switch tok.TokValue {
			case "BETWEEN":
				low := o.expr(4)
				o.expect("kw", "AND")
				left = oForm{not + "BETWEEN", []oNode{left, low, o.expr(4)}}
			case "IN":
				left = oForm{not + "IN", []oNode{left, o.inTarget()}}
			case "LIKE", "GLOB", "REGEXP", "MATCH":
				args := []oNode{left, o.expr(4)}
				if o.accept("kw", "ESCAPE") {
					args = append(args, oAtom("ESCAPE"), o.expr(4))
				}
				left = oForm{not + tok.TokValue, args}
			default:
				panic(errNoParse)
			}
		default:
			return left
		}
	}
}

// primary parses an atom or a prefix operator applied to an expression.  A
// prefix operator always applies, whatever the bound.
func (o *oracle) primary() oNode {
	tok := o.peek()
	switch tok.TokType {
	case "number", "string", "blob", "param":
		o.advance()
		return oAtom(tok.TokValue)
	case "kw":
		switch tok.TokValue {
		case "NOT":
			o.advance()
			return oForm{"NOT", []oNode{o.expr(3)}}
		case "NULL", "CURRENT_TIME", "CURRENT_DATE", "CURRENT_TIMESTAMP":
			o.advance()
			return oAtom(tok.TokValue)
		}
	case "op":
		switch tok.TokValue {
		case "-", "+", "~":
			o.advance()
			return oForm{tok.TokValue, []oNode{o.expr(10)}}
		case "(":
			o.advance()
			items := o.exprList()
			o.expect("op", ")")
			switch len(items) {
			case 0:
				panic(errNoParse)
			case 1:
				return oParen{items[0]}
			}
			return oForm{"ROW", items}
		}
	case "ckw", "ident", "qident":
		if tok.TokType == "ckw" && (tok.TokValue == "TRUE" || tok.TokValue == "FALSE") {
			o.advance()
			return oAtom(tok.TokValue)
		}
		name := o.name()
		if o.accept("op", "(") {
			var args []oNode
			if o.accept("op", "*") {
				args = append(args, oAtom("*"))
			}
			if o.accept("kw", "DISTINCT") {
				args = append(args, oAtom("DISTINCT"))
			}
			args = append(args, o.exprList()...)
			o.expect("op", ")")
			return oForm{name, args}
		}
		for n := 1; n < 3 && o.accept("op", "."); n++ {
			name += "." + o.name()
		}
		return oAtom(name)
	}
	panic(errNoParse)
}

// exprList parses a possibly empty comma-separated list of expressions,
// each under no bound.
func (o *oracle) exprList() []oNode {
	if o.is("op", ")") {
		return nil
	}
	items := []oNode{o.expr(0)}
	for o.accept("op", ",") {
		items = append(items, o.expr(0))
	}
	return items
}

func (o *oracle) inTarget() oNode {
	if o.accept("op", "(") {
		var items []string
		for _, item := range o.exprList() {
			items = append(items, item.String())
		}
		o.expect("op", ")")
		return oAtom("(" + strings.Join(items, " ") + ")")
	}
	name := o.name()
	if o.accept("op", ".") {
		name += "." + o.name()
	}
	return oAtom(name)
}

func (o *oracle) name() string {
	tok := o.peek()
	switch tok.TokType {
	case "ident", "qident", "ckw":
		o.advance()
		return tok.TokValue
	}
	panic(errNoParse)
}

// tokens tokenises src and returns the tokens with the stream, rewound.
func tokens(t testing.TB, src string) ([]grammar.SimpleToken, *grammar.SimpleTokenStream) {
	stream, err := Tokenise(src)
	if err != nil {
		t.Fatalf("%q: %s", src, err)
	}
	var toks []grammar.SimpleToken
	for tok := stream.Next(); tok.Type() != grammar.EOF.TokType; tok = stream.Next() {
		toks = append(toks, *tok.(*grammar.SimpleToken))
	}
	stream.Restore(0)
	return toks, stream
}

// checkExpr generates an expression from data, parses it with the grammar
// and the oracle and compares; it reports whether the expression parsed.
func checkExpr(t testing.TB, data []byte) bool {
	c := &chooser{data: data}
	var words []string
	if c.pick(4) == 0 {
		for n := c.pick(12); n > 0; n-- {
			words = append(words, genVocab[c.pick(len(genVocab))])
		}
	} else {
		genExpr(c, 1+c.pick(4), &words)
	}
	src := strings.Join(words, " ")
	toks, stream := tokens(t, src)
	want, wantOK := oracleParse(toks)
	var expr Expr
	perr := grammar.Parse(&expr, stream, grammar.Exhaust)
	got, gotOK := "", false
	if perr == nil {
		got, gotOK = lisp(&expr), true
	}
	if gotOK != wantOK || got != want {
		describe := func(s string, ok bool, err *grammar.ParseError) string {
			if ok {
				return s
			}
			if err != nil {
				return "FAIL: " + err.Error()
			}
			return "FAIL"
		}
		t.Errorf("%s\n grammar: %s\n  oracle: %s", src, describe(got, gotOK, perr), describe(want, wantOK, nil))
	}
	return gotOK
}

func FuzzExpr(f *testing.F) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		data := make([]byte, 1+rng.Intn(40))
		rng.Read(data)
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) { checkExpr(t, data) })
}

func TestExprOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	successes := 0
	for i := 0; i < 20000 && !t.Failed(); i++ {
		data := make([]byte, rng.Intn(64))
		rng.Read(data)
		if checkExpr(t, data) {
			successes++
		}
	}
	t.Logf("%d expressions, %d parsed", 20000, successes)
	if successes < 3000 {
		t.Errorf("only %d successful parses, the inputs are not varied enough", successes)
	}
}

// ---- Statements ----

var corpusData struct {
	once  sync.Once
	stmts [][]string // the token values of each corpus statement
	vocab []string   // the distinct tokens of the corpus
}

func loadCorpus(t testing.TB) {
	corpusData.once.Do(func() {
		seen := map[string]bool{}
		for _, src := range corpus(t) {
			toks, _ := tokens(t, src)
			var words []string
			for _, tok := range toks {
				words = append(words, tok.TokValue)
				if !seen[tok.TokValue] {
					seen[tok.TokValue] = true
					corpusData.vocab = append(corpusData.vocab, tok.TokValue)
				}
			}
			corpusData.stmts = append(corpusData.stmts, words)
		}
	})
}

// genStmt returns the tokens of a random statement: a token soup, or a
// corpus statement with a few random edits.
func genStmt(c *chooser) []string {
	vocab := corpusData.vocab
	var words []string
	if c.pick(3) == 0 {
		for n := c.pick(30); n > 0; n-- {
			words = append(words, vocab[c.pick(len(vocab))])
		}
		return words
	}
	words = append(words, corpusData.stmts[c.pick(len(corpusData.stmts))]...)
	for n := c.pick(3); n > 0 && len(words) > 0; n-- {
		i := c.pick(len(words))
		switch c.pick(5) {
		case 0:
			words = append(words[:i], words[i+1:]...)
		case 1:
			words = append(words[:i], append([]string{vocab[c.pick(len(vocab))]}, words[i:]...)...)
		case 2:
			words[i] = vocab[c.pick(len(vocab))]
		case 3:
			words = append(words[:i], append([]string{words[i]}, words[i:]...)...)
		case 4:
			j := c.pick(len(words))
			words[i], words[j] = words[j], words[i]
		}
	}
	return words
}

var simpleTokenType = reflect.TypeOf(grammar.SimpleToken{})

// treeTokens collects the tokens stored in a parse tree, in field order.
func treeTokens(v reflect.Value, out *[]grammar.SimpleToken) {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if !v.IsNil() {
			treeTokens(v.Elem(), out)
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			treeTokens(v.Index(i), out)
		}
	case reflect.Struct:
		if v.Type() == simpleTokenType {
			if typ := v.FieldByName("TokType").String(); typ != "" {
				*out = append(*out, grammar.SimpleToken{TokType: typ, TokValue: v.FieldByName("TokValue").String()})
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			treeTokens(v.Field(i), out)
		}
	}
}

// checkStmt generates a statement from data and parses it; if it parses,
// the tokens in the tree must be a subsequence of the input (not every
// token is stored: Match fields drop theirs).  It reports whether the
// statement parsed.
func checkStmt(t testing.TB, data []byte) bool {
	loadCorpus(t)
	src := strings.Join(genStmt(&chooser{data: data}), " ")
	toks, stream := tokens(t, src)
	var stmt Stmt
	if grammar.Parse(&stmt, stream, grammar.Exhaust) != nil {
		return false
	}
	var leaves []grammar.SimpleToken
	treeTokens(reflect.ValueOf(&stmt), &leaves)
	j := 0
	for _, tok := range toks {
		if j < len(leaves) && leaves[j].TokType == tok.TokType && leaves[j].TokValue == tok.TokValue {
			j++
		}
	}
	if j < len(leaves) {
		t.Errorf("%s\n the tokens of the tree are not those of the input in order; tree: %v", src, leaves)
	}
	return true
}

func FuzzStmt(f *testing.F) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		data := make([]byte, 1+rng.Intn(40))
		rng.Read(data)
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) { checkStmt(t, data) })
}

func TestStmtRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	successes := 0
	for i := 0; i < 5000 && !t.Failed(); i++ {
		data := make([]byte, rng.Intn(64))
		rng.Read(data)
		if checkStmt(t, data) {
			successes++
		}
	}
	t.Logf("%d statements, %d parsed", 5000, successes)
	if successes < 500 {
		t.Errorf("only %d successful parses, the inputs are not varied enough", successes)
	}
}

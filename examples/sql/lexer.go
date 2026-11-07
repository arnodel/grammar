// Package sql is a parser for a SQLite-like dialect of SQL: the SELECT,
// INSERT, UPDATE and DELETE statements with their expressions, joins,
// subqueries, common table expressions and window functions.
//
// It is an example of a grammar large enough to show how the pieces of the
// grammar package fit together on a real language, and of how to handle a
// language that has several dialects: the differences between SQL dialects
// are mostly lexical (quoting, comments, placeholders, which words are
// reserved), so they live in the tokeniser, and the grammar is written
// against dialect-neutral token types.
package sql

import (
	"strings"

	"github.com/arnodel/grammar"
)

// Token is the token type used throughout the grammar.
type Token = grammar.SimpleToken

// The token types seen by the grammar.
//
// Words are classified by Tokenise into three types.  Reserved words become
// "kw" tokens and the other words that the grammar gives a meaning to in some
// contexts become "ckw" tokens, both with their value folded to upper case so
// that a tag like `tok:"kw,SELECT"` matches whatever the case of the input.
// Everything else is an "ident" token with its value as written.  Contextual
// keywords can be used as names (a column called "first", a table called
// "rows") because the Name rule accepts them, while reserved words cannot,
// which is what lets `FROM t WHERE` know that WHERE is not an alias for t.
//
// Which words are reserved is the kind of thing that varies between dialects.
// The list here is roughly SQLite's, with the join keywords and WINDOW added
// to it, which SQLite only manages to leave unreserved by looking ahead in
// the lexer.
var reservedWords = wordSet(`
	ALL AND AS BETWEEN BY CASE CAST COLLATE CROSS CURRENT_DATE CURRENT_TIME
	CURRENT_TIMESTAMP DEFAULT DELETE DISTINCT ELSE END ESCAPE EXCEPT EXISTS
	FROM FULL GLOB GROUP HAVING IN INDEXED INNER INSERT INTERSECT INTO IS
	ISNULL JOIN LEFT LIKE LIMIT MATCH NATURAL NOT NOTNULL NULL ON OR ORDER
	OUTER REGEXP RETURNING RIGHT SELECT SET THEN UNION UPDATE USING VALUES
	WHEN WHERE WINDOW WITH
`)

var contextualWords = wordSet(`
	ABORT ASC CONFLICT CURRENT DESC DO EXCLUDE FAIL FALSE FILTER FIRST
	FOLLOWING GROUPS IGNORE LAST MATERIALIZED NO NOTHING NULLS OFFSET OTHERS
	OVER PARTITION PRECEDING RANGE RECURSIVE REPLACE ROLLBACK ROW ROWS TIES
	TRUE UNBOUNDED
`)

func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(words) {
		set[w] = true
	}
	return set
}

var tokenise = grammar.SimpleTokeniser([]grammar.TokenDef{
	{Ptn: `\s+`},
	{Ptn: `--[^\n]*`},
	{Ptn: `/\*(?:[^*]|\*+[^*/])*\*+/`},
	{Name: "word", Ptn: `[a-zA-Z_][a-zA-Z0-9_$]*`}, // classified by Tokenise
	{Name: "qident", Ptn: `"(?:[^"]|"")*"` + "|`(?:[^`]|``)*`" + `|\[[^\]]*\]`},
	{Name: "string", Ptn: `'(?:[^']|'')*'`},
	{Name: "blob", Ptn: `[xX]'[0-9a-fA-F]*'`}, // longer than the word "x", so it wins
	{Name: "number", Ptn: `0[xX][0-9a-fA-F]+|(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][-+]?[0-9]+)?`},
	{Name: "param", Ptn: `\?[0-9]*|[:@$][a-zA-Z_][a-zA-Z0-9_$]*`},
	// The "|" and "||" operators get their own token types because a "|" in
	// a tok tag separates alternatives, so `tok:"op,|"` cannot be written.
	{Name: "concat", Ptn: `\|\|`},
	{Name: "bitor", Ptn: `\|`},
	{Name: "op", Ptn: `->>|->|<<|>>|<=|>=|==|!=|<>|[-+*/%&<>=~(),.;]`},
})

// Tokenise splits SQL source into tokens, classifying words as described
// above.
func Tokenise(src string) (*grammar.SimpleTokenStream, error) {
	stream, err := tokenise(src)
	if err != nil {
		return nil, err
	}
	// The tokens of the stream are *SimpleToken, so they can be amended in
	// place.
	for tok := stream.Next(); tok.Type() != grammar.EOF.TokType; tok = stream.Next() {
		if word, ok := tok.(*grammar.SimpleToken); ok && word.TokType == "word" {
			classifyWord(word)
		}
	}
	stream.Restore(0)
	return stream, nil
}

func classifyWord(tok *grammar.SimpleToken) {
	upper := strings.ToUpper(tok.TokValue)
	switch {
	case reservedWords[upper]:
		tok.TokType, tok.TokValue = "kw", upper
	case contextualWords[upper]:
		tok.TokType, tok.TokValue = "ckw", upper
	default:
		tok.TokType = "ident"
	}
}

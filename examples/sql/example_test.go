package sql

import (
	"fmt"

	"github.com/arnodel/grammar"
)

func Example() {
	toks, _ := Tokenise(`
		WITH recent AS (SELECT * FROM orders WHERE placed > date('now', '-7 days'))
		SELECT customer, count(*) AS n, sum(total) FILTER (WHERE paid)
		FROM recent NATURAL JOIN customers
		GROUP BY customer HAVING n > 1
		ORDER BY 3 DESC NULLS LAST LIMIT 10;
	`)
	var script Script
	if err := grammar.Parse(&script, toks, grammar.Exhaust); err != nil {
		fmt.Println(err)
		return
	}
	sel := script.Stmts[0].Select
	fmt.Println("CTEs:", len(sel.With.CTEs))
	fmt.Println("columns:", len(sel.First.Select.Columns))
	fmt.Println("joins:", len(sel.First.Select.From.Join.Joins))
	// lisp (in expr_test.go) prints an expression tree as an S-expression.
	fmt.Println("having:", lisp(&sel.First.Select.GroupBy.Having.Expr))

	// Errors are reported at the furthest point the parser reached, with what
	// could have come next there.
	toks, _ = Tokenise(`SELECT a FROM t WHERE x = 1 AND`)
	fmt.Println(grammar.Parse(&script, toks, grammar.Exhaust))

	// Output:
	// CTEs: 1
	// columns: 3
	// joins: 1
	// having: (> n 1)
	// Ln 1, Col 32: EOF with value "EOF": expected token with type number or string or blob or param or ident or ckw or qident, or value "NOT" or "-" or "+" or "~" or "NULL" or "TRUE" or "FALSE" or "CURRENT_TIME" or "CURRENT_DATE" or "CURRENT_TIMESTAMP" or "CAST" or "CASE" or "EXISTS" or "("
}

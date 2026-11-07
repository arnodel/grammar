-- Statements that must parse.  They are separated by a line holding only ";".
SELECT 1
;
select a, b as c, d e, t.* from t where a = 1 and b > 2 or not c
;
SELECT DISTINCT a FROM t
;
SELECT ALL * FROM t
;
SELECT count(*), sum(x) AS total, avg(DISTINCT y) FROM t GROUP BY z HAVING count(*) > 1
;
SELECT * FROM t ORDER BY a DESC NULLS LAST, b ASC, c COLLATE NOCASE LIMIT 10 OFFSET 5
;
SELECT * FROM t LIMIT 10, 5
;
SELECT a FROM t UNION SELECT a FROM u UNION ALL SELECT a FROM v INTERSECT SELECT a FROM w EXCEPT SELECT a FROM x ORDER BY 1
;
WITH RECURSIVE c(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM c WHERE n < 10) SELECT n FROM c
;
WITH a AS MATERIALIZED (SELECT 1), b AS NOT MATERIALIZED (SELECT 2) SELECT * FROM a, b
;
SELECT a.x, b.y FROM a LEFT OUTER JOIN b ON a.id = b.a_id, c NATURAL JOIN d USING (z)
;
SELECT * FROM a INNER JOIN b ON a.x = b.x CROSS JOIN c RIGHT JOIN d ON d.y = c.y FULL OUTER JOIN e
;
SELECT * FROM (SELECT 1 AS x) AS sub, (a JOIN b ON a.id = b.id), json_each(t.data) j
;
SELECT * FROM main.t AS u INDEXED BY idx WHERE u.x = 1
;
SELECT * FROM t NOT INDEXED
;
SELECT * FROM "quoted table", `backticks`, [brackets]
;
SELECT row_number() OVER (PARTITION BY dept ORDER BY salary) FROM emp
;
SELECT sum(x) OVER (ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) FROM t
;
SELECT sum(x) OVER (w RANGE BETWEEN 1 PRECEDING AND 2 FOLLOWING EXCLUDE NO OTHERS), sum(x) OVER (GROUPS CURRENT ROW EXCLUDE TIES) FROM t WINDOW w AS (ORDER BY y)
;
SELECT first, last, rows, offset, range FROM t WHERE first > last
;
SELECT count(*) FILTER (WHERE x > 1) OVER w FROM t WINDOW w AS (PARTITION BY y)
;
SELECT CASE WHEN a THEN 1 WHEN b THEN 2 ELSE 3 END, CASE x WHEN 1 THEN 'one' END FROM t
;
SELECT CAST(x AS VARCHAR(10)), CAST(y AS UNSIGNED BIG INT), CAST(z AS DECIMAL(10, -2))
;
SELECT -1 + ~2 * 3 || 'a' -> '$.x' ->> 'y', +x, a & 1 | 2 << 3 >> 4, 5 % 2, 1 / 2
;
SELECT x IS NOT DISTINCT FROM y, x ISNULL, x NOTNULL, x NOT NULL, x IS NULL, x IS NOT NULL
;
SELECT x NOT BETWEEN 1 AND 2, y NOT LIKE 'a%' ESCAPE '\', z GLOB 'a*', z REGEXP 'a.*', z MATCH 'a'
;
SELECT x IN (1, 2, 3), y NOT IN (SELECT y FROM t), z IN other_table, w IN ()
;
SELECT EXISTS (SELECT 1 FROM t), NOT EXISTS (SELECT 1 FROM t)
;
SELECT (1, 2) = (3, 4), (SELECT max(x) FROM t), (a + b) * c
;
SELECT ?, ?1, :name, @name, $name, x'DEADBEEF', 0x1F, 1.5e10, .5, 1., 'it''s', NULL, TRUE, FALSE, CURRENT_TIMESTAMP
;
SELECT /* a comment */ 1 -- and another
;
VALUES (1, 2), (3, 4)
;
INSERT INTO t (a, b) VALUES (1, 'x'), (2, 'y')
;
INSERT INTO main.t AS new (a) SELECT a FROM u WHERE a > 1 RETURNING a, new.b AS c
;
INSERT OR IGNORE INTO t DEFAULT VALUES
;
REPLACE INTO t VALUES (1)
;
INSERT INTO t (a, b) VALUES (1, 'x') ON CONFLICT (a) WHERE a > 0 DO UPDATE SET b = excluded.b, (c, d) = (SELECT 1, 2) WHERE t.b IS NOT NULL
;
INSERT INTO t VALUES (1) ON CONFLICT DO NOTHING
;
WITH v(x) AS (VALUES (1), (2)) INSERT INTO t SELECT * FROM v
;
UPDATE t SET a = a + 1, b = 'x' WHERE a IN (SELECT a FROM u) RETURNING *
;
UPDATE OR REPLACE t AS u INDEXED BY i SET (a, b) = (1, 2) FROM v, w WHERE u.id = v.id
;
WITH c AS (SELECT 1) UPDATE t SET a = (SELECT * FROM c)
;
DELETE FROM t
;
DELETE FROM t AS u WHERE u.x NOT BETWEEN 1 AND 2 AND y NOT LIKE 'a%' RETURNING id
;
WITH d AS (SELECT id FROM t WHERE old) DELETE FROM t WHERE id IN d
;

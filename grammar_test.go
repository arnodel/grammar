package grammar

import (
	"errors"
	"reflect"
	"testing"
)

func str(s string) SimpleToken {
	return SimpleToken{tokType: "string", tokValue: s}
}

func num(s string) SimpleToken {
	return SimpleToken{tokType: "number", tokValue: s}
}

func bl(s string) SimpleToken {
	return SimpleToken{tokType: "bool", tokValue: s}
}

func null() SimpleToken {
	return SimpleToken{tokType: "null", tokValue: "null"}
}

func op(s string) SimpleToken {
	return SimpleToken{tokType: "op", tokValue: s}
}

func s(toks ...Token) *SliceTokenStream {
	return &SliceTokenStream{tokens: toks}
}

func TestParse(t *testing.T) {
	type args struct {
		dest interface{}
		t    TokenStream
	}
	tests := []struct {
		name    string
		args    args
		want    interface{}
		wantErr error
	}{
		{
			name: "a string",
			args: args{
				dest: new(Json),
				t:    s(str("hello")),
			},
			want: &Json{
				String: &String{Token: str("hello")},
			},
		},
		{
			name: "an empty array",
			args: args{
				dest: new(Json),
				t:    s(op("["), op("]")),
			},
			want: &Json{
				Array: &Array{
					Open:  op("["),
					Close: op("]"),
				},
			},
		},
		{
			name: "an array with one string",
			args: args{
				dest: new(Json),
				t:    s(op("["), str("item"), op("]")),
			},
			want: &Json{
				Array: &Array{
					Open: op("["),
					ArrayBody: &ArrayBody{
						First: Json{String: &String{Token: str("item")}},
					},
					Close: op("]"),
				},
			},
		},
		{
			name: "an array with two strings",
			args: args{
				dest: new(Json),
				t:    s(op("["), str("item1"), op(","), str("item2"), op("]")),
			},
			want: &Json{
				Array: &Array{
					Open: op("["),
					ArrayBody: &ArrayBody{
						First: Json{String: &String{Token: str("item1")}},
						Items: []ArrayItem{
							{
								Comma: op(","),
								Value: Json{String: &String{str("item2")}},
							},
						},
					},
					Close: op("]"),
				},
			},
		},
		{
			name: "a dict with one item",
			args: args{
				dest: new(Json),
				t:    s(op("{"), str("key1"), op(":"), str("val1"), op("}")),
			},
			want: &Json{
				Dict: &Dict{
					Open: op("{"),
					DictBody: &DictBody{
						First: KeyValue{
							Key:   String{str("key1")},
							Colon: op(":"),
							Value: Json{String: &String{str("val1")}},
						},
					},
					Close: op("}"),
				},
			},
		},
		{
			name: "a dict with two items",
			args: args{
				dest: new(Json),
				t:    s(op("{"), str("key1"), op(":"), str("val1"), op(","), str("key2"), op(":"), num("123"), op("}")),
			},
			want: &Json{
				Dict: &Dict{
					Open: op("{"),
					DictBody: &DictBody{
						First: KeyValue{
							Key:   String{str("key1")},
							Colon: op(":"),
							Value: Json{String: &String{str("val1")}},
						},
						Items: []DictItem{
							{
								Comma: op(","),
								KeyValue: KeyValue{
									Key:   String{str("key2")},
									Colon: op(":"),
									Value: Json{Number: &Number{num("123")}},
								},
							},
						},
					},
					Close: op("}"),
				},
			},
		},
		{
			name: "invalid",
			args: args{
				dest: new(Json),
				t:    s(op("{"), op("]")),
			},
			wantErr: &ParseError{
				Err:   errors.New("expected token of type string"),
				Pos:   1,
				Token: op("]"),
			},
		},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Parse(tt.args.dest, tt.args.t); !reflect.DeepEqual(tt.wantErr, err) {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want != nil && !reflect.DeepEqual(tt.want, tt.args.dest) {
				t.Errorf("Parse() dest = %v, want = %v", tt.args.dest, tt.want)
			}
		})
	}
}

func TestParse2(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want interface{}
	}{
		{
			name: "simple dict",
			in:   `{"x": 2, "y": "abc"}`,
			want: map[string]interface{}{
				"x": 2.0,
				"y": "abc",
			},
		},
		{
			name: "nested data",
			in:   `[1, "xyz", true, {"hello": ["a", "b", 42], "bye": null}]`,
			want: []interface{}{
				1.0,
				"xyz",
				true,
				map[string]interface{}{
					"hello": []interface{}{"a", "b", 42.0},
					"bye":   nil,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, err := TokeniseJsonString(tt.in)
			if err != nil {
				t.Fatalf("Error tokenising: %s", err)
			}
			dest := new(Json)
			err = Parse(dest, stream)
			if err != nil {
				t.Fatalf("Error parsing: %s", err)
			}
			out := dest.Compile()
			if !reflect.DeepEqual(out, tt.want) {
				t.Errorf("out = %v, want = %v", out, tt.want)
			}
		})
	}
}

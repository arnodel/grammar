package grammar

import (
	"reflect"
	"testing"
)

func Test_calcRuleDef(t *testing.T) {
	type args struct {
		tp reflect.Type
	}

	type Rule1 struct{}
	type Rule2 struct{}

	type OneOfRule struct {
		OneOf
		*Rule1
		*Rule2
	}

	type SeqRule struct {
		Seq
		A Rule1
		B []Rule2
	}

	type SepRule struct{}

	type SeqRuleWithSeparator struct {
		Seq
		Separator SepRule
		Items     []Rule1
	}

	type Token struct{}

	type RuleWithTokens struct {
		Seq
		Separator Token   `tok:"op,,"`
		Items     []Token `tok:"int"`
	}

	tests := []struct {
		name    string
		args    args
		want    *RuleDef
		wantErr bool
	}{
		{
			name: "OneOf",
			args: args{tp: reflect.TypeOf(OneOfRule{})},
			want: &RuleDef{
				Name:  "OneOfRule",
				OneOf: true,
				Fields: []RuleField{
					{
						Index: 1,
						Name:  "Rule1",
						FieldType: FieldType{
							BaseType: reflect.TypeOf(Rule1{}),
							Pointer:  true,
						},
					},
					{
						Index: 2,
						Name:  "Rule2",
						FieldType: FieldType{
							BaseType: reflect.TypeOf(Rule2{}),
							Pointer:  true,
						},
					},
				},
			},
		},
		{
			name: "Sequence",
			args: args{tp: reflect.TypeOf(SeqRule{})},
			want: &RuleDef{
				Name: "SeqRule",
				Fields: []RuleField{
					{
						Index: 1,
						Name:  "A",
						FieldType: FieldType{
							BaseType: reflect.TypeOf(Rule1{}),
						},
					},
					{
						Index: 2,
						Name:  "B",
						FieldType: FieldType{
							BaseType: reflect.TypeOf(Rule2{}),
							Array:    true,
						},
					},
				},
			},
		},
		{
			name: "Sequence with separator",
			args: args{tp: reflect.TypeOf(SeqRuleWithSeparator{})},
			want: &RuleDef{
				Name: "SeqRuleWithSeparator",
				Separator: &RuleField{
					Index: 1,
					Name:  "Separator",
					FieldType: FieldType{
						BaseType: reflect.TypeOf(SepRule{}),
					},
				},
				Fields: []RuleField{
					{
						Index: 2,
						Name:  "Items",
						FieldType: FieldType{
							BaseType: reflect.TypeOf(Rule1{}),
							Array:    true,
						},
					},
				},
			},
		},
		{
			name: "Rule with tokens",
			args: args{tp: reflect.TypeOf(RuleWithTokens{})},
			want: &RuleDef{
				Name: "RuleWithTokens",
				Separator: &RuleField{
					Index: 1,
					Name:  "Separator",
					FieldType: FieldType{
						BaseType: reflect.TypeOf(Token{}),
					},
					ParseOptions: ParseOptions{
						TokenParseOptions: []TokenParseOptions{
							{
								TokenType:  "op",
								TokenValue: ",",
							},
						},
					},
				},
				Fields: []RuleField{
					{
						Index: 2,
						Name:  "Items",
						FieldType: FieldType{
							BaseType: reflect.TypeOf(Token{}),
							Array:    true,
						},
						ParseOptions: ParseOptions{
							TokenParseOptions: []TokenParseOptions{
								{
									TokenType: "int",
								},
							},
						},
					},
				},
			},
		},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calcRuleDef(tt.args.tp)
			if (err != nil) != tt.wantErr {
				t.Errorf("calcRuleDef() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calcRuleDef() = %v,\nwant %v", got, tt.want)
			}
		})
	}
}

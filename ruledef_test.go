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
		Seq `drop:"spc"`
		A   Rule1
		B   []Rule2
	}

	type SeqRuleWithSeparator struct {
		Seq
		Items []Rule1 `sep:"op,,"`
	}

	type Token struct{}

	type RuleWithTokens struct {
		Seq
		Items []Token `tok:"int" sep:"op,,"`
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
				DropOptions: TokenOptions{
					TokenParseOptions: []TokenParseOptions{
						{
							TokenType: "spc",
						},
					},
				},
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
				Fields: []RuleField{
					{
						Index: 1,
						Name:  "Items",
						FieldType: FieldType{
							BaseType: reflect.TypeOf(Rule1{}),
							Array:    true,
						},
						SepOptions: TokenOptions{
							TokenParseOptions: []TokenParseOptions{
								{
									TokenType:  "op",
									TokenValue: ",",
								},
							},
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
				Fields: []RuleField{
					{
						Index: 1,
						Name:  "Items",
						FieldType: FieldType{
							BaseType: reflect.TypeOf(Token{}),
							Array:    true,
						},
						TokenOptions: TokenOptions{
							TokenParseOptions: []TokenParseOptions{
								{
									TokenType: "int",
								},
							},
						},
						SepOptions: TokenOptions{
							TokenParseOptions: []TokenParseOptions{
								{
									TokenType:  "op",
									TokenValue: ",",
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

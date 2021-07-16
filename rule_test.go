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
		Rule
		A Rule1
		B []Rule2
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
				t.Errorf("calcRuleDef() = %v, want %v", got, tt.want)
			}
		})
	}
}

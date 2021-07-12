package grammar

type Json struct {
	OneOf
	*Number
	*String
	*Null
	*Bool
	*Array
	*Dict
}

type Number struct {
	Token SimpleToken `tok:"number"`
}

type String struct {
	Token SimpleToken `tok:"string"`
}

type Null struct {
	Token SimpleToken `tok:",null"`
}

type Bool struct {
	Token SimpleToken `tok:"bool"`
}

type Array struct {
	Open SimpleToken `tok:",["`
	*ArrayBody
	Close SimpleToken `tok:",]"`
}

type ArrayBody struct {
	First Json
	Items []ArrayItem
}

type ArrayItem struct {
	Comma SimpleToken `tok:",,"`
	Value Json
}

type Dict struct {
	Open SimpleToken `tok:",{"`
	*DictBody
	Close SimpleToken `tok:",}"`
}

type DictBody struct {
	First KeyValue
	Items []DictItem
}

type KeyValue struct {
	Key   String
	Colon SimpleToken `tok:",:"`
	Value Json
}

type DictItem struct {
	Comma SimpleToken `tok:",,"`
	KeyValue
}

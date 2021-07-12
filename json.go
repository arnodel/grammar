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
	Token `tok:"number"`
}

type String struct {
	Token `tok:"string"`
}

type Null struct {
	Token `tok:",null"`
}

type Bool struct {
	Token `tok:"bool"`
}

type Array struct {
	Open Token `tok:",["`
	*ArrayBody
	Close Token `tok:",]"`
}

type ArrayBody struct {
	First Json
	Items []ArrayItem
}

type ArrayItem struct {
	Comma Token `tok:",,"`
	Value Json
}

type Dict struct {
	Open Token `tok:",{"`
	*DictBody
	Close Token `tok:",}"`
}

type DictBody struct {
	First KeyValue
	Items []DictItem
}

type KeyValue struct {
	Key   String
	Colon Token `tok:",:"`
	Value Json
}

type DictItem struct {
	Comma Token `tok:",,"`
	KeyValue
}

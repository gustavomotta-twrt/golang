package models

type CustomFieldValue struct {
	FieldId      string
	FieldName    string
	FieldType    string
	Value        any
	EnumOptionId string
}

type CustomFieldDefinition struct {
	Id          string
	Name        string
	Type        string
	EnumOptions []CustomFieldEnumOption
}

type CustomFieldEnumOption struct {
	Id   string
	Name string
}

type CustomFieldMapping struct {
	SourceFieldId   string
	SourceFieldName string
	SourceFieldType string
	DestFieldId     string
	DestFieldName   string
	DestFieldType   string
	Degraded        bool
	OptionMappings  []CustomFieldOptionMapping
}

type CustomFieldOptionMapping struct {
	SourceOptionId   string
	SourceOptionName string
	DestOptionId     string
	DestOptionName   string
}

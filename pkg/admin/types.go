package admin

// CreateCollectionRequest is the request body for creating a collection.
type CreateCollectionRequest struct {
	Name   string        `json:"name" binding:"required"`
	Fields []FieldDef    `json:"fields" binding:"required,min=1"`
}

// FieldDef defines a field for creating or altering tables.
type FieldDef struct {
	Name       string      `json:"name" binding:"required"`
	Type       string      `json:"type" binding:"required"`
	Required   bool        `json:"required,omitempty"`
	Unique     bool        `json:"unique,omitempty"`
	Primary    bool        `json:"primary,omitempty"`
	Default    interface{} `json:"default,omitempty"`
	MaxLength  *int        `json:"max_length,omitempty"`
	Precision  *int        `json:"precision,omitempty"`
	Scale      *int        `json:"scale,omitempty"`
	References *ForeignRef `json:"references,omitempty"`
}

// ForeignRef defines a foreign key reference.
type ForeignRef struct {
	Table    string `json:"table" binding:"required"`
	Column   string `json:"column" binding:"required"`
	OnDelete string `json:"on_delete,omitempty"` // CASCADE, SET NULL, RESTRICT, NO ACTION
	OnUpdate string `json:"on_update,omitempty"`
}

// AddFieldRequest is the request body for adding a field.
type AddFieldRequest struct {
	Field FieldDef `json:"field" binding:"required"`
}

// AlterFieldRequest is the request body for altering a field.
type AlterFieldRequest struct {
	Type      *string     `json:"type,omitempty"`
	Required  *bool       `json:"required,omitempty"`
	Unique    *bool       `json:"unique,omitempty"`
	Default   interface{} `json:"default,omitempty"`
	MaxLength *int        `json:"max_length,omitempty"`
	NewName   *string     `json:"new_name,omitempty"`
}

// CollectionInfo represents collection information for admin endpoints.
type CollectionInfo struct {
	Name       string      `json:"name"`
	TableName  string      `json:"table_name"`
	Enabled    bool        `json:"enabled"`
	Fields     []FieldInfo `json:"fields"`
	PrimaryKey string      `json:"primary_key"`
}

// FieldInfo represents field information for admin endpoints.
type FieldInfo struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	PostgresType string  `json:"postgres_type"`
	Required     bool    `json:"required"`
	Unique       bool    `json:"unique"`
	Primary      bool    `json:"primary"`
	Default      *string `json:"default,omitempty"`
	MaxLength    *int    `json:"max_length,omitempty"`
}

// TypeMapping maps abstract types to PostgreSQL types.
var TypeMapping = map[string]string{
	"uuid":      "UUID",
	"string":    "VARCHAR(255)",
	"text":      "TEXT",
	"int":       "INTEGER",
	"bigint":    "BIGINT",
	"float":     "DOUBLE PRECISION",
	"decimal":   "DECIMAL",
	"boolean":   "BOOLEAN",
	"date":      "DATE",
	"time":      "TIME",
	"timestamp": "TIMESTAMP",
	"json":      "JSONB",
	"binary":    "BYTEA",
}

// GetPostgresType converts an abstract type to PostgreSQL type.
func GetPostgresType(abstractType string, maxLength *int, precision, scale *int) string {
	switch abstractType {
	case "string":
		if maxLength != nil && *maxLength > 0 {
			return "VARCHAR(" + itoa(*maxLength) + ")"
		}
		return "VARCHAR(255)"
	case "decimal":
		if precision != nil && scale != nil {
			return "DECIMAL(" + itoa(*precision) + "," + itoa(*scale) + ")"
		}
		return "DECIMAL(10,2)"
	default:
		if pgType, ok := TypeMapping[abstractType]; ok {
			return pgType
		}
		return "TEXT"
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

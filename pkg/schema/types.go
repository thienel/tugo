package schema

import "time"

// Collection represents a discovered database table/collection.
type Collection struct {
	ID         string    `db:"id" json:"id"`
	Name       string    `db:"name" json:"name"`             // API name (e.g., "products")
	TableName  string    `db:"table_name" json:"table_name"` // Actual table name (e.g., "api_products")
	Enabled    bool      `db:"enabled" json:"enabled"`
	Fields     []Field   `json:"fields,omitempty"`
	PrimaryKey string    `json:"primary_key,omitempty"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}

// Field represents a column in a table.
type Field struct {
	ID              string          `db:"id" json:"id"`
	CollectionID    string          `db:"collection_id" json:"collection_id"`
	Name            string          `db:"name" json:"name"`
	DataType        string          `db:"data_type" json:"data_type"`
	PostgresType    string          `json:"postgres_type,omitempty"`
	IsNullable      bool            `db:"is_nullable" json:"is_nullable"`
	IsUnique        bool            `db:"is_unique" json:"is_unique"`
	IsPrimaryKey    bool            `json:"is_primary_key"`
	DefaultValue    *string         `db:"default_value" json:"default_value,omitempty"`
	MaxLength       *int            `db:"max_length" json:"max_length,omitempty"`
	Precision       *int            `db:"precision" json:"precision,omitempty"`
	Scale           *int            `db:"scale" json:"scale,omitempty"`
	ForeignKey      *ForeignKeyInfo `json:"foreign_key,omitempty"`
	ValidationRules map[string]any  `json:"validation_rules,omitempty"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
}

// ForeignKeyInfo holds foreign key relationship information.
type ForeignKeyInfo struct {
	Table    string `json:"table"`
	Column   string `json:"column"`
	OnDelete string `json:"on_delete,omitempty"`
	OnUpdate string `json:"on_update,omitempty"`
}

// Relationship represents a relationship between collections.
type Relationship struct {
	ID                  string `db:"id" json:"id"`
	CollectionID        string `db:"collection_id" json:"collection_id"`
	FieldName           string `db:"field_name" json:"field_name"`
	RelatedCollectionID string `db:"related_collection_id" json:"related_collection_id"`
	RelatedCollection   string `json:"related_collection,omitempty"`             // API name
	RelationshipType    string `db:"relationship_type" json:"relationship_type"` // many_to_one, one_to_many, many_to_many
	JunctionTable       string `db:"junction_table" json:"junction_table,omitempty"`
	JunctionField       string `db:"junction_field" json:"junction_field,omitempty"`
}

// PostgresColumnInfo represents raw column info from PostgreSQL.
type PostgresColumnInfo struct {
	TableName     string  `db:"table_name"`
	ColumnName    string  `db:"column_name"`
	DataType      string  `db:"data_type"`
	UDTName       string  `db:"udt_name"`
	IsNullable    string  `db:"is_nullable"`
	ColumnDefault *string `db:"column_default"`
	CharMaxLength *int    `db:"character_maximum_length"`
	NumPrecision  *int    `db:"numeric_precision"`
	NumScale      *int    `db:"numeric_scale"`
}

// PostgresForeignKeyInfo represents raw FK info from PostgreSQL.
type PostgresForeignKeyInfo struct {
	ConstraintName    string `db:"constraint_name"`
	TableName         string `db:"table_name"`
	ColumnName        string `db:"column_name"`
	ForeignTableName  string `db:"foreign_table_name"`
	ForeignColumnName string `db:"foreign_column_name"`
	DeleteRule        string `db:"delete_rule"`
	UpdateRule        string `db:"update_rule"`
}

// PostgresPrimaryKeyInfo represents primary key info.
type PostgresPrimaryKeyInfo struct {
	TableName  string `db:"table_name"`
	ColumnName string `db:"column_name"`
}

// PostgresUniqueInfo represents unique constraint info.
type PostgresUniqueInfo struct {
	TableName  string `db:"table_name"`
	ColumnName string `db:"column_name"`
}

// DataTypeMap maps PostgreSQL types to abstract types.
var DataTypeMap = map[string]string{
	"uuid":                        "uuid",
	"int2":                        "int",
	"int4":                        "int",
	"int8":                        "int",
	"smallint":                    "int",
	"integer":                     "int",
	"bigint":                      "int",
	"float4":                      "float",
	"float8":                      "float",
	"real":                        "float",
	"double precision":            "float",
	"numeric":                     "decimal",
	"decimal":                     "decimal",
	"varchar":                     "string",
	"character varying":           "string",
	"char":                        "string",
	"character":                   "string",
	"text":                        "string",
	"bool":                        "boolean",
	"boolean":                     "boolean",
	"timestamp":                   "timestamp",
	"timestamp without time zone": "timestamp",
	"timestamp with time zone":    "timestamp",
	"timestamptz":                 "timestamp",
	"date":                        "date",
	"time":                        "time",
	"time without time zone":      "time",
	"time with time zone":         "time",
	"timetz":                      "time",
	"json":                        "json",
	"jsonb":                       "json",
	"bytea":                       "binary",
	"interval":                    "interval",
}

// MapPostgresType converts a PostgreSQL type to an abstract type.
func MapPostgresType(pgType string) string {
	if abstractType, ok := DataTypeMap[pgType]; ok {
		return abstractType
	}
	return "string" // default to string for unknown types
}

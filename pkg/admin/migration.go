package admin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MigrationGenerator generates SQL migration files.
type MigrationGenerator struct {
	outputDir string
}

// NewMigrationGenerator creates a new migration generator.
func NewMigrationGenerator(outputDir string) *MigrationGenerator {
	return &MigrationGenerator{outputDir: outputDir}
}

// Migration represents a generated migration.
type Migration struct {
	Version   string
	Name      string
	UpSQL     string
	DownSQL   string
	UpPath    string
	DownPath  string
}

// GenerateCreateTable generates a create table migration.
func (g *MigrationGenerator) GenerateCreateTable(req CreateCollectionRequest) (*Migration, error) {
	tableName := req.Name
	if !strings.HasPrefix(tableName, "api_") {
		tableName = "api_" + tableName
	}

	// Build UP migration
	var upBuilder strings.Builder
	upBuilder.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", tableName))

	var columns []string
	var constraints []string

	for _, field := range req.Fields {
		colDef := buildColumnDef(field)
		columns = append(columns, "    "+colDef)

		if field.References != nil {
			fkName := fmt.Sprintf("fk_%s_%s", tableName, field.Name)
			fk := fmt.Sprintf("    CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s)",
				fkName, field.Name, field.References.Table, field.References.Column)
			if field.References.OnDelete != "" {
				fk += " ON DELETE " + field.References.OnDelete
			}
			if field.References.OnUpdate != "" {
				fk += " ON UPDATE " + field.References.OnUpdate
			}
			constraints = append(constraints, fk)
		}
	}

	upBuilder.WriteString(strings.Join(columns, ",\n"))
	if len(constraints) > 0 {
		upBuilder.WriteString(",\n")
		upBuilder.WriteString(strings.Join(constraints, ",\n"))
	}
	upBuilder.WriteString("\n);\n")

	// Add indexes for unique columns
	for _, field := range req.Fields {
		if field.Unique && !field.Primary {
			idxName := fmt.Sprintf("idx_%s_%s", tableName, field.Name)
			upBuilder.WriteString(fmt.Sprintf("\nCREATE UNIQUE INDEX %s ON %s(%s);\n", idxName, tableName, field.Name))
		}
	}

	// Build DOWN migration
	downSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", tableName)

	return g.createMigration("create_"+req.Name, upBuilder.String(), downSQL)
}

// GenerateAddColumn generates an add column migration.
func (g *MigrationGenerator) GenerateAddColumn(tableName string, field FieldDef) (*Migration, error) {
	if !strings.HasPrefix(tableName, "api_") {
		tableName = "api_" + tableName
	}

	colDef := buildColumnDef(field)
	upSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;\n", tableName, colDef)

	if field.Unique {
		idxName := fmt.Sprintf("idx_%s_%s", tableName, field.Name)
		upSQL += fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s(%s);\n", idxName, tableName, field.Name)
	}

	if field.References != nil {
		fkName := fmt.Sprintf("fk_%s_%s", tableName, field.Name)
		upSQL += fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s)",
			tableName, fkName, field.Name, field.References.Table, field.References.Column)
		if field.References.OnDelete != "" {
			upSQL += " ON DELETE " + field.References.OnDelete
		}
		if field.References.OnUpdate != "" {
			upSQL += " ON UPDATE " + field.References.OnUpdate
		}
		upSQL += ";\n"
	}

	downSQL := fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s;\n", tableName, field.Name)

	return g.createMigration(fmt.Sprintf("add_%s_to_%s", field.Name, tableName), upSQL, downSQL)
}

// GenerateDropColumn generates a drop column migration.
func (g *MigrationGenerator) GenerateDropColumn(tableName, columnName string) (*Migration, error) {
	if !strings.HasPrefix(tableName, "api_") {
		tableName = "api_" + tableName
	}

	upSQL := fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s;\n", tableName, columnName)
	downSQL := "-- Cannot automatically restore dropped column\n-- Manual intervention required\n"

	return g.createMigration(fmt.Sprintf("drop_%s_from_%s", columnName, tableName), upSQL, downSQL)
}

// GenerateAlterColumn generates an alter column migration.
func (g *MigrationGenerator) GenerateAlterColumn(tableName, columnName string, req AlterFieldRequest) (*Migration, error) {
	if !strings.HasPrefix(tableName, "api_") {
		tableName = "api_" + tableName
	}

	var upParts []string
	var downParts []string

	if req.Type != nil {
		pgType := GetPostgresType(*req.Type, req.MaxLength, nil, nil)
		upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tableName, columnName, pgType))
		downParts = append(downParts, "-- Type change requires manual rollback")
	}

	if req.Required != nil {
		if *req.Required {
			upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", tableName, columnName))
			downParts = append(downParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", tableName, columnName))
		} else {
			upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", tableName, columnName))
			downParts = append(downParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", tableName, columnName))
		}
	}

	if req.Default != nil {
		upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %v;", tableName, columnName, req.Default))
		downParts = append(downParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", tableName, columnName))
	}

	if req.NewName != nil && *req.NewName != columnName {
		upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", tableName, columnName, *req.NewName))
		downParts = append(downParts, fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", tableName, *req.NewName, columnName))
	}

	upSQL := strings.Join(upParts, "\n") + "\n"
	downSQL := strings.Join(downParts, "\n") + "\n"

	return g.createMigration(fmt.Sprintf("alter_%s_in_%s", columnName, tableName), upSQL, downSQL)
}

// GenerateDropTable generates a drop table migration.
func (g *MigrationGenerator) GenerateDropTable(tableName string) (*Migration, error) {
	if !strings.HasPrefix(tableName, "api_") {
		tableName = "api_" + tableName
	}

	upSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", tableName)
	downSQL := "-- Cannot automatically restore dropped table\n-- Manual intervention required\n"

	return g.createMigration("drop_"+tableName, upSQL, downSQL)
}

// createMigration creates a migration with the given name and SQL.
func (g *MigrationGenerator) createMigration(name, upSQL, downSQL string) (*Migration, error) {
	version := time.Now().Format("20060102150405")

	migration := &Migration{
		Version: version,
		Name:    name,
		UpSQL:   upSQL,
		DownSQL: downSQL,
	}

	if g.outputDir != "" {
		// Ensure directory exists
		if err := os.MkdirAll(g.outputDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create migrations directory: %w", err)
		}

		// Write UP migration
		upFileName := fmt.Sprintf("%s_%s.up.sql", version, name)
		migration.UpPath = filepath.Join(g.outputDir, upFileName)
		if err := os.WriteFile(migration.UpPath, []byte(upSQL), 0644); err != nil {
			return nil, fmt.Errorf("failed to write UP migration: %w", err)
		}

		// Write DOWN migration
		downFileName := fmt.Sprintf("%s_%s.down.sql", version, name)
		migration.DownPath = filepath.Join(g.outputDir, downFileName)
		if err := os.WriteFile(migration.DownPath, []byte(downSQL), 0644); err != nil {
			return nil, fmt.Errorf("failed to write DOWN migration: %w", err)
		}
	}

	return migration, nil
}

// buildColumnDef builds a column definition string.
func buildColumnDef(field FieldDef) string {
	var parts []string

	parts = append(parts, field.Name)
	parts = append(parts, GetPostgresType(field.Type, field.MaxLength, field.Precision, field.Scale))

	if field.Primary {
		if field.Type == "uuid" {
			parts = append(parts, "PRIMARY KEY DEFAULT gen_random_uuid()")
		} else {
			parts = append(parts, "PRIMARY KEY")
		}
	} else {
		if field.Required {
			parts = append(parts, "NOT NULL")
		}
		if field.Default != nil {
			parts = append(parts, fmt.Sprintf("DEFAULT %v", formatDefault(field.Default)))
		}
	}

	return strings.Join(parts, " ")
}

// formatDefault formats a default value for SQL.
func formatDefault(value interface{}) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	default:
		return fmt.Sprintf("%v", v)
	}
}

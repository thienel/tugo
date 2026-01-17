package schema

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/thienel/tugo/pkg/apperror"
	"go.uber.org/zap"
)

// DiscoveryMode defines how collections are discovered.
type DiscoveryMode string

const (
	DiscoveryModePrefix  DiscoveryMode = "prefix"
	DiscoveryModeConfig  DiscoveryMode = "config"
	DiscoveryModeHybrid  DiscoveryMode = "hybrid"
)

// ManagerConfig holds configuration for the schema manager.
type ManagerConfig struct {
	Mode         DiscoveryMode
	Prefix       string
	AutoDiscover bool
	Blacklist    []string
	Config       map[string]CollectionConfig
}

// CollectionConfig holds per-collection configuration.
type CollectionConfig struct {
	Enabled      bool
	PublicFields []string
}

// Manager handles schema discovery and metadata management.
type Manager struct {
	db           *sqlx.DB
	introspector *Introspector
	config       ManagerConfig
	logger       *zap.SugaredLogger

	collections    map[string]*Collection // keyed by API name
	relationships  map[string][]Relationship
	mu             sync.RWMutex
	lastRefresh    time.Time
}

// NewManager creates a new schema manager.
func NewManager(db *sqlx.DB, config ManagerConfig, logger *zap.SugaredLogger) *Manager {
	if config.Prefix == "" {
		config.Prefix = "api_"
	}
	if config.Mode == "" {
		config.Mode = DiscoveryModePrefix
	}
	if config.Blacklist == nil {
		config.Blacklist = []string{}
	}
	if config.Config == nil {
		config.Config = make(map[string]CollectionConfig)
	}

	return &Manager{
		db:            db,
		introspector:  NewIntrospector(db),
		config:        config,
		logger:        logger,
		collections:   make(map[string]*Collection),
		relationships: make(map[string][]Relationship),
	}
}

// Refresh discovers and caches all collections.
func (m *Manager) Refresh(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("Refreshing schema...")

	// Get all tables matching prefix
	tables, err := m.introspector.GetTables(ctx, m.config.Prefix)
	if err != nil {
		m.logger.Errorw("Failed to get tables", "error", err)
		return err
	}

	m.logger.Infow("Found tables", "count", len(tables))

	// Clear existing collections
	m.collections = make(map[string]*Collection)
	m.relationships = make(map[string][]Relationship)

	// Process each table
	for _, tableName := range tables {
		if m.isBlacklisted(tableName) {
			m.logger.Debugw("Skipping blacklisted table", "table", tableName)
			continue
		}

		apiName := m.tableToAPIName(tableName)
		enabled := m.isEnabled(tableName, apiName)

		if !enabled {
			m.logger.Debugw("Skipping disabled collection", "collection", apiName)
			continue
		}

		collection, err := m.introspectTable(ctx, tableName, apiName)
		if err != nil {
			m.logger.Errorw("Failed to introspect table", "table", tableName, "error", err)
			continue
		}

		m.collections[apiName] = collection
		m.logger.Debugw("Discovered collection", "collection", apiName, "fields", len(collection.Fields))
	}

	// Build relationships
	if err := m.buildRelationships(ctx); err != nil {
		m.logger.Errorw("Failed to build relationships", "error", err)
	}

	m.lastRefresh = time.Now()
	m.logger.Infow("Schema refresh complete", "collections", len(m.collections))

	return nil
}

// GetCollection returns a collection by API name.
func (m *Manager) GetCollection(name string) (*Collection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	collection, ok := m.collections[name]
	if !ok {
		return nil, apperror.ErrCollectionNotFound.WithMessagef("Collection '%s' not found", name)
	}
	return collection, nil
}

// GetCollections returns all collections.
func (m *Manager) GetCollections() []*Collection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Collection, 0, len(m.collections))
	for _, c := range m.collections {
		result = append(result, c)
	}
	return result
}

// GetRelationships returns relationships for a collection.
func (m *Manager) GetRelationships(collectionName string) []Relationship {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.relationships[collectionName]
}

// GetRelationship returns a specific relationship by field name.
func (m *Manager) GetRelationship(collectionName, fieldName string) (*Relationship, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rels := m.relationships[collectionName]
	for _, rel := range rels {
		if rel.FieldName == fieldName {
			return &rel, true
		}
	}
	return nil, false
}

// HasCollection checks if a collection exists.
func (m *Manager) HasCollection(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.collections[name]
	return ok
}

// introspectTable gathers full metadata for a table.
func (m *Manager) introspectTable(ctx context.Context, tableName, apiName string) (*Collection, error) {
	// Get columns
	columns, err := m.introspector.GetColumns(ctx, tableName)
	if err != nil {
		return nil, err
	}

	// Get primary keys
	pks, err := m.introspector.GetPrimaryKeys(ctx, tableName)
	if err != nil {
		return nil, err
	}
	pkSet := make(map[string]bool)
	for _, pk := range pks {
		pkSet[pk.ColumnName] = true
	}

	// Get unique columns
	uniques, err := m.introspector.GetUniqueColumns(ctx, tableName)
	if err != nil {
		return nil, err
	}
	uniqueSet := make(map[string]bool)
	for _, u := range uniques {
		uniqueSet[u.ColumnName] = true
	}

	// Get foreign keys
	fks, err := m.introspector.GetForeignKeys(ctx, tableName)
	if err != nil {
		return nil, err
	}
	fkMap := make(map[string]PostgresForeignKeyInfo)
	for _, fk := range fks {
		fkMap[fk.ColumnName] = fk
	}

	// Build fields
	fields := make([]Field, 0, len(columns))
	var primaryKey string
	for _, col := range columns {
		field := Field{
			ID:           uuid.New().String(),
			Name:         col.ColumnName,
			DataType:     MapPostgresType(col.UDTName),
			PostgresType: col.UDTName,
			IsNullable:   col.IsNullable == "YES",
			IsUnique:     uniqueSet[col.ColumnName],
			IsPrimaryKey: pkSet[col.ColumnName],
			DefaultValue: col.ColumnDefault,
			MaxLength:    col.CharMaxLength,
			Precision:    col.NumPrecision,
			Scale:        col.NumScale,
			CreatedAt:    time.Now(),
		}

		if fk, ok := fkMap[col.ColumnName]; ok {
			field.ForeignKey = &ForeignKeyInfo{
				Table:    fk.ForeignTableName,
				Column:   fk.ForeignColumnName,
				OnDelete: fk.DeleteRule,
				OnUpdate: fk.UpdateRule,
			}
		}

		if field.IsPrimaryKey {
			primaryKey = field.Name
		}

		fields = append(fields, field)
	}

	return &Collection{
		ID:         uuid.New().String(),
		Name:       apiName,
		TableName:  tableName,
		Enabled:    true,
		Fields:     fields,
		PrimaryKey: primaryKey,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

// buildRelationships creates relationship metadata from foreign keys.
func (m *Manager) buildRelationships(ctx context.Context) error {
	for apiName, collection := range m.collections {
		rels := make([]Relationship, 0)

		for _, field := range collection.Fields {
			if field.ForeignKey == nil {
				continue
			}

			// Find the related collection
			relatedAPIName := m.tableToAPIName(field.ForeignKey.Table)
			relatedCollection, ok := m.collections[relatedAPIName]
			if !ok {
				continue
			}

			rel := Relationship{
				ID:                  uuid.New().String(),
				CollectionID:        collection.ID,
				FieldName:           field.Name,
				RelatedCollectionID: relatedCollection.ID,
				RelatedCollection:   relatedAPIName,
				RelationshipType:    "many_to_one",
			}
			rels = append(rels, rel)
		}

		m.relationships[apiName] = rels
	}

	return nil
}

// tableToAPIName converts a table name to an API name.
func (m *Manager) tableToAPIName(tableName string) string {
	return strings.TrimPrefix(tableName, m.config.Prefix)
}

// apiNameToTable converts an API name to a table name.
func (m *Manager) apiNameToTable(apiName string) string {
	return m.config.Prefix + apiName
}

// isBlacklisted checks if a table is blacklisted.
func (m *Manager) isBlacklisted(tableName string) bool {
	for _, b := range m.config.Blacklist {
		if b == tableName {
			return true
		}
	}
	return false
}

// isEnabled determines if a collection should be enabled based on config.
func (m *Manager) isEnabled(tableName, apiName string) bool {
	switch m.config.Mode {
	case DiscoveryModePrefix:
		// Check override config first
		if cfg, ok := m.config.Config[apiName]; ok {
			return cfg.Enabled
		}
		if cfg, ok := m.config.Config[tableName]; ok {
			return cfg.Enabled
		}
		// Default based on AutoDiscover
		return m.config.AutoDiscover

	case DiscoveryModeConfig:
		// Only enable if explicitly configured
		if cfg, ok := m.config.Config[apiName]; ok {
			return cfg.Enabled
		}
		if cfg, ok := m.config.Config[tableName]; ok {
			return cfg.Enabled
		}
		return false

	case DiscoveryModeHybrid:
		// Check override config first
		if cfg, ok := m.config.Config[apiName]; ok {
			return cfg.Enabled
		}
		if cfg, ok := m.config.Config[tableName]; ok {
			return cfg.Enabled
		}
		// Default to enabled for prefix matches
		return true

	default:
		return m.config.AutoDiscover
	}
}

// GetPublicFields returns the public fields for a collection.
func (m *Manager) GetPublicFields(collectionName string) []string {
	if cfg, ok := m.config.Config[collectionName]; ok {
		return cfg.PublicFields
	}
	return nil
}

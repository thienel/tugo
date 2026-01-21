package collection

import (
	"context"

	"github.com/thienel/tugo/pkg/apperror"
	"github.com/thienel/tugo/pkg/query"
	"github.com/thienel/tugo/pkg/response"
	"github.com/thienel/tugo/pkg/schema"
	"github.com/thienel/tugo/pkg/validation"
	"go.uber.org/zap"
)

// Service provides business logic for collection operations.
type Service struct {
	repo          *Repository
	schemaManager *schema.Manager
	validator     *validation.ValidatorRegistry
	logger        *zap.SugaredLogger
}

// NewService creates a new collection service.
func NewService(repo *Repository, schemaManager *schema.Manager, logger *zap.SugaredLogger) *Service {
	return &Service{
		repo:          repo,
		schemaManager: schemaManager,
		logger:        logger,
	}
}

// SetValidator sets the validator registry.
func (s *Service) SetValidator(v *validation.ValidatorRegistry) {
	s.validator = v
}

// ListParams holds parameters for listing items.
type ListParams struct {
	CollectionName string
	QueryParams    map[string][]string
	Expand         []string
}

// List retrieves a list of items with filtering, sorting, and pagination.
func (s *Service) List(ctx context.Context, params ListParams) (*ListResponse, error) {
	collection, err := s.schemaManager.GetCollection(params.CollectionName)
	if err != nil {
		return nil, err
	}

	// Get allowed field names for validation
	fieldNames := getFieldNames(collection.Fields)

	// Parse filters
	filterParser := query.NewFilterParser(fieldNames)
	filters, err := filterParser.Parse(params.QueryParams)
	if err != nil {
		return nil, err
	}

	// Parse sorts
	sortParser := query.NewSortParser(fieldNames)
	sortParam := ""
	if sortStrs, ok := params.QueryParams["sort"]; ok && len(sortStrs) > 0 {
		sortParam = sortStrs[0]
	}
	sorts, err := sortParser.Parse(sortParam)
	if err != nil {
		return nil, err
	}

	// Default sort by primary key if not specified
	if len(sorts) == 0 && collection.PrimaryKey != "" {
		sorts = query.DefaultSort(collection.PrimaryKey)
	}

	// Parse pagination
	pagination := query.ParsePagination(params.QueryParams)

	// Execute query
	result, err := s.repo.List(ctx, collection, ListOptions{
		Filters:    filters,
		Sorts:      sorts,
		Pagination: pagination,
	})
	if err != nil {
		return nil, err
	}

	// Handle expand
	if len(params.Expand) > 0 {
		if err := s.expandItems(ctx, collection, result.Items, params.Expand); err != nil {
			s.logger.Warnw("Failed to expand relationships", "error", err)
		}
	}

	return &ListResponse{
		Items: result.Items,
		Pagination: response.NewPagination(
			pagination.Page,
			pagination.Limit,
			result.Total,
		),
	}, nil
}

// Get retrieves a single item by ID.
func (s *Service) Get(ctx context.Context, collectionName string, id any, expand []string) (map[string]any, error) {
	collection, err := s.schemaManager.GetCollection(collectionName)
	if err != nil {
		return nil, err
	}

	item, err := s.repo.GetByID(ctx, collection, id)
	if err != nil {
		return nil, err
	}

	// Handle expand
	if len(expand) > 0 {
		items := []map[string]any{item}
		if err := s.expandItems(ctx, collection, items, expand); err != nil {
			s.logger.Warnw("Failed to expand relationships", "error", err)
		}
	}

	return item, nil
}

// Create creates a new item.
func (s *Service) Create(ctx context.Context, collectionName string, data map[string]any) (map[string]any, error) {
	collection, err := s.schemaManager.GetCollection(collectionName)
	if err != nil {
		return nil, err
	}

	// Filter out unknown fields
	filteredData := filterFields(data, collection.Fields)

	// Validate data
	if s.validator != nil {
		if validationErr := s.validator.Validate(ctx, collectionName, filteredData); validationErr != nil {
			return nil, apperror.ErrValidation.WithMessage(validationErr.Error()).WithDetails(validationErr.Errors)
		}
	}

	return s.repo.Create(ctx, collection, filteredData)
}

// Update updates an existing item.
func (s *Service) Update(ctx context.Context, collectionName string, id any, data map[string]any) (map[string]any, error) {
	collection, err := s.schemaManager.GetCollection(collectionName)
	if err != nil {
		return nil, err
	}

	// Filter out unknown fields
	filteredData := filterFields(data, collection.Fields)

	// Validate data (for updates, we only validate provided fields - skip required check)
	if s.validator != nil {
		if validationErr := s.validator.ValidatePartial(ctx, collectionName, filteredData); validationErr != nil {
			return nil, apperror.ErrValidation.WithMessage(validationErr.Error()).WithDetails(validationErr.Errors)
		}
	}

	return s.repo.Update(ctx, collection, id, filteredData)
}

// Delete removes an item by ID.
func (s *Service) Delete(ctx context.Context, collectionName string, id any) error {
	collection, err := s.schemaManager.GetCollection(collectionName)
	if err != nil {
		return err
	}

	return s.repo.Delete(ctx, collection, id)
}

// expandItems expands relationships in items.
func (s *Service) expandItems(ctx context.Context, collection *schema.Collection, items []map[string]any, expand []string) error {
	for _, expandField := range expand {
		rel, ok := s.schemaManager.GetRelationship(collection.Name, expandField+"_id")
		if !ok {
			// Try without _id suffix
			rel, ok = s.schemaManager.GetRelationship(collection.Name, expandField)
			if !ok {
				continue
			}
		}

		relatedCollection, err := s.schemaManager.GetCollection(rel.RelatedCollection)
		if err != nil {
			continue
		}

		// Collect foreign key values
		fkField := rel.FieldName
		ids := make([]any, 0)
		for _, item := range items {
			if fkValue, ok := item[fkField]; ok && fkValue != nil {
				ids = append(ids, fkValue)
			}
		}

		if len(ids) == 0 {
			continue
		}

		// Fetch related items
		relatedItems, err := s.repo.GetRelated(ctx, relatedCollection, relatedCollection.PrimaryKey, ids)
		if err != nil {
			return err
		}

		// Merge related items into main items
		// The expand field name is the field without _id suffix
		expandKey := expandField
		for _, item := range items {
			if fkValue, ok := item[fkField]; ok && fkValue != nil {
				normalizedFK := normalizeValue(fkValue)
				if related, ok := relatedItems[normalizedFK]; ok {
					item[expandKey] = related
				}
			}
		}
	}

	return nil
}

// ListResponse holds the response for list operations.
type ListResponse struct {
	Items      []map[string]any
	Pagination *response.Pagination
}

// getFieldNames extracts field names from a slice of fields.
func getFieldNames(fields []schema.Field) []string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	return names
}

// filterFields removes fields that don't exist in the schema.
func filterFields(data map[string]any, fields []schema.Field) map[string]any {
	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[f.Name] = true
	}

	filtered := make(map[string]any)
	for k, v := range data {
		if fieldSet[k] {
			filtered[k] = v
		}
	}
	return filtered
}

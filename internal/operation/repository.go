package operation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	opaqueRefPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	digestPattern          = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	windowsAbsPattern      = regexp.MustCompile(`(?i)^[a-z]:[\\/]`)
	absPathFragmentPattern = regexp.MustCompile(`(?i)(^|[[:space:]])(/[[:graph:]]+|[a-z]:[\\/][[:graph:]]*)`)
)

type Store struct {
	db      *gorm.DB
	now     func() time.Time
	writeMu sync.Mutex
}

func Open(root string) (*Store, error) {
	dir := filepath.Join(root, ".pinax", "api")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	databasePath := filepath.Join(dir, "operations.sqlite")
	db, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	return OpenDB(db)
}

// OpenExisting opens an existing operation ledger without creating one for a
// read-only status lookup on an older vault.
func OpenExisting(root string) (*Store, error) {
	databasePath := filepath.Join(root, ".pinax", "api", "operations.sqlite")
	if _, err := os.Stat(databasePath); err != nil {
		if os.IsNotExist(err) {
			return nil, operationError(CodeNotFound, "Operation was not found", nil)
		}
		return nil, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	db, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	return OpenDB(db)
}

func OpenDB(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, operationError(CodeStoreUnavailable, "Operation store is unavailable", nil)
	}
	store := &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
	if err := store.Migrate(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return operationError(CodeStoreUnavailable, "Operation store is unavailable", nil)
	}
	if err := s.db.WithContext(ctx).AutoMigrate(&OperationRow{}, &IdempotencyRow{}); err != nil {
		return operationError(CodeStoreUnavailable, "Operation store migration failed", err)
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) CreateAccepted(ctx context.Context, request CreateRequest) (CreateResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := validateCreateRequest(request); err != nil {
		return CreateResult{}, err
	}
	var created OperationRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, found, err := findBinding(ctx, tx, request)
		if err != nil {
			return err
		}
		if found {
			if !sameBinding(existing, request) {
				return operationError(CodeIdempotencyConflict, "Idempotency binding conflicts with an existing operation", nil)
			}
			if err := tx.WithContext(ctx).First(&created, "operation_id = ?", existing.OperationID).Error; err != nil {
				return err
			}
			return errReplay
		}

		now := s.now()
		created = OperationRow{
			OperationID:     request.OperationID,
			CapabilityID:    request.CapabilityID,
			BindingID:       request.BindingID,
			PrincipalDigest: request.PrincipalDigest,
			ScopeDigest:     request.ScopeDigest,
			RequestDigest:   request.RequestDigest,
			Status:          StatusAccepted,
			RevisionBefore:  strings.TrimSpace(request.RevisionBefore),
			AcceptedAt:      now,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.WithContext(ctx).Create(&created).Error; err != nil {
			return err
		}
		binding := IdempotencyRow{
			PrincipalDigest: request.PrincipalDigest,
			ScopeDigest:     request.ScopeDigest,
			IdempotencyKey:  request.IdempotencyKey,
			CapabilityID:    request.CapabilityID,
			BindingID:       request.BindingID,
			OperationID:     request.OperationID,
			RequestDigest:   request.RequestDigest,
			CreatedAt:       now,
		}
		return tx.WithContext(ctx).Create(&binding).Error
	})
	if errors.Is(err, errReplay) {
		return CreateResult{Operation: created, Replay: true}, nil
	}
	if err == nil {
		return CreateResult{Operation: created}, nil
	}
	if IsCode(err, CodeIdempotencyConflict) {
		return CreateResult{}, err
	}
	// A concurrent creator can win the unique idempotency index between the
	// lookup and insert. Re-read the binding and return the original operation
	// only when every immutable binding field still matches.
	if replay, replayErr := s.lookupReplay(ctx, request); replayErr == nil {
		return replay, nil
	} else if IsCode(replayErr, CodeIdempotencyConflict) {
		return CreateResult{}, replayErr
	}
	return CreateResult{}, operationError(CodeStoreUnavailable, "Operation store could not persist the accepted record", err)
}

// FindReplay performs a read-only lookup for an existing immutable operation
// binding. It lets mutation adapters return durable outcomes before evaluating
// preconditions that may no longer hold after a successful mutation.
func (s *Store) FindReplay(ctx context.Context, request CreateRequest) (CreateResult, bool, error) {
	if err := validateCreateRequest(request); err != nil {
		return CreateResult{}, false, err
	}
	binding, found, err := findBinding(ctx, s.db, request)
	if err != nil {
		return CreateResult{}, false, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	if !found {
		var byOperation OperationRow
		if err := s.db.WithContext(ctx).First(&byOperation, "operation_id = ?", request.OperationID).Error; err == nil {
			return CreateResult{}, false, operationError(CodeIdempotencyConflict, "Operation identity conflicts with an existing operation", nil)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return CreateResult{}, false, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
		}
		return CreateResult{}, false, nil
	}
	if !sameBinding(binding, request) {
		return CreateResult{}, false, operationError(CodeIdempotencyConflict, "Idempotency binding conflicts with an existing operation", nil)
	}
	var row OperationRow
	if err := s.db.WithContext(ctx).First(&row, "operation_id = ?", binding.OperationID).Error; err != nil {
		return CreateResult{}, false, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	return CreateResult{Operation: row, Replay: true}, true, nil
}

var errReplay = errors.New("operation replay")

func (s *Store) lookupReplay(ctx context.Context, request CreateRequest) (CreateResult, error) {
	binding, found, err := findBinding(ctx, s.db, request)
	if err != nil {
		return CreateResult{}, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	if !found {
		var byOperation OperationRow
		if err := s.db.WithContext(ctx).First(&byOperation, "operation_id = ?", request.OperationID).Error; err == nil {
			return CreateResult{}, operationError(CodeIdempotencyConflict, "Operation identity conflicts with an existing operation", nil)
		}
		return CreateResult{}, operationError(CodeStoreUnavailable, "Operation store is unavailable", nil)
	}
	if !sameBinding(binding, request) {
		return CreateResult{}, operationError(CodeIdempotencyConflict, "Idempotency binding conflicts with an existing operation", nil)
	}
	var operation OperationRow
	if err := s.db.WithContext(ctx).First(&operation, "operation_id = ?", binding.OperationID).Error; err != nil {
		return CreateResult{}, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	return CreateResult{Operation: operation, Replay: true}, nil
}

func (s *Store) Get(ctx context.Context, operationID string) (OperationRow, error) {
	if !opaqueRefPattern.MatchString(operationID) {
		return OperationRow{}, operationError(CodeNotFound, "Operation was not found", nil)
	}
	var operation OperationRow
	if err := s.db.WithContext(ctx).First(&operation, "operation_id = ?", operationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return OperationRow{}, operationError(CodeNotFound, "Operation was not found", nil)
		}
		return OperationRow{}, operationError(CodeStoreUnavailable, "Operation store is unavailable", err)
	}
	return operation, nil
}

func (s *Store) GetAuthorized(ctx context.Context, operationID, principalDigest, scopeDigest string) (OperationRow, error) {
	operation, err := s.Get(ctx, operationID)
	if err != nil {
		return OperationRow{}, err
	}
	if operation.PrincipalDigest != principalDigest || operation.ScopeDigest != scopeDigest {
		return OperationRow{}, operationError(CodeNotFound, "Operation was not found", nil)
	}
	return operation, nil
}

func (s *Store) StartApplying(ctx context.Context, operationID string) (OperationRow, error) {
	return s.transition(ctx, operationID, StatusApplying, Outcome{})
}

// StartApplyingWithOutcome persists bounded preparation evidence before the
// domain mutation starts. This closes the crash window where reconciliation
// would otherwise know that an operation was applying but not which durable
// resource identity it intended to create.
func (s *Store) StartApplyingWithOutcome(ctx context.Context, operationID string, outcome Outcome) (OperationRow, error) {
	return s.transition(ctx, operationID, StatusApplying, outcome)
}

func (s *Store) RetryApplying(ctx context.Context, operationID string) (OperationRow, error) {
	return s.transition(ctx, operationID, StatusApplying, Outcome{})
}

func (s *Store) CompleteSucceeded(ctx context.Context, operationID string, outcome Outcome) (OperationRow, error) {
	return s.transition(ctx, operationID, StatusSucceeded, outcome)
}

func (s *Store) CompleteFailed(ctx context.Context, operationID string, outcome Outcome) (OperationRow, error) {
	return s.transition(ctx, operationID, StatusFailed, outcome)
}

func (s *Store) RequireReconcile(ctx context.Context, operationID string, outcome Outcome) (OperationRow, error) {
	return s.transition(ctx, operationID, StatusReconcileRequired, outcome)
}

func (s *Store) transition(ctx context.Context, operationID string, target Status, outcome Outcome) (OperationRow, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	current, err := s.Get(ctx, operationID)
	if err != nil {
		return OperationRow{}, err
	}
	if !CanTransition(current.Status, target, current.Retryable, current.ReplaySafe) {
		return OperationRow{}, operationError(CodeIllegalTransition, "Operation state transition is not allowed", nil)
	}
	result, err := boundedJSON(outcome.Result, MaxResultBytes)
	if err != nil {
		return OperationRow{}, err
	}
	receipt, err := boundedJSON(outcome.Receipt, MaxReceiptBytes)
	if err != nil {
		return OperationRow{}, err
	}
	now := s.now()
	if err := validateOutcome(outcome); err != nil {
		return OperationRow{}, err
	}
	updates := map[string]any{"status": target, "updated_at": now}
	if len(result) > 0 {
		updates["result_json"] = result
	}
	if len(receipt) > 0 {
		updates["receipt_json"] = receipt
	}
	for key, value := range map[string]string{
		"receipt_ref": outcome.ReceiptRef, "resource_ref": outcome.ResourceRef,
		"revision_after": outcome.RevisionAfter, "error_code": outcome.ErrorCode,
		"error_message": outcome.ErrorMessage,
	} {
		if strings.TrimSpace(value) != "" {
			updates[key] = strings.TrimSpace(value)
		}
	}
	if target == StatusFailed {
		updates["retryable"] = outcome.Retryable
		updates["replay_safe"] = outcome.ReplaySafe
	}
	if target == StatusApplying {
		updates["applying_at"] = now
		updates["completed_at"] = nil
		updates["error_code"] = ""
		updates["error_message"] = ""
		updates["retryable"] = false
		updates["replay_safe"] = false
	}
	if StatusTerminal(target) {
		updates["completed_at"] = now
	}
	if target == StatusReconcileRequired {
		updates["reconcile_count"] = gorm.Expr("reconcile_count + ?", 1)
	}
	resultDB := s.db.WithContext(ctx).Model(&OperationRow{}).
		Where("operation_id = ? AND status = ?", operationID, current.Status).
		Updates(updates)
	if resultDB.Error != nil {
		return OperationRow{}, operationError(CodeStoreUnavailable, "Operation state could not be persisted", resultDB.Error)
	}
	if resultDB.RowsAffected != 1 {
		return OperationRow{}, operationError(CodeIllegalTransition, "Operation state changed concurrently", nil)
	}
	return s.Get(ctx, operationID)
}

func findBinding(ctx context.Context, db *gorm.DB, request CreateRequest) (IdempotencyRow, bool, error) {
	var binding IdempotencyRow
	err := db.WithContext(ctx).
		Where("principal_digest = ? AND scope_digest = ? AND idempotency_key = ?", request.PrincipalDigest, request.ScopeDigest, request.IdempotencyKey).
		First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return IdempotencyRow{}, false, nil
	}
	return binding, err == nil, err
}

func sameBinding(binding IdempotencyRow, request CreateRequest) bool {
	return binding.CapabilityID == request.CapabilityID && binding.BindingID == request.BindingID && binding.OperationID == request.OperationID && binding.RequestDigest == request.RequestDigest
}

func validateCreateRequest(request CreateRequest) error {
	if !opaqueRefPattern.MatchString(request.OperationID) || !opaqueRefPattern.MatchString(request.IdempotencyKey) {
		return operationError(CodeIdentityRequired, "Operation identity and idempotency key are required", nil)
	}
	if strings.TrimSpace(request.CapabilityID) == "" || strings.TrimSpace(request.BindingID) == "" {
		return operationError(CodeIdentityRequired, "Operation capability and binding are required", nil)
	}
	if !digestPattern.MatchString(request.PrincipalDigest) || !digestPattern.MatchString(request.ScopeDigest) || !digestPattern.MatchString(request.RequestDigest) {
		return operationError(CodeIdentityRequired, "Operation identity digests are invalid", nil)
	}
	if err := validateLedgerRef(request.RevisionBefore, 160); err != nil {
		return err
	}
	return nil
}

// ValidateCreateRequest lets mutation adapters reject incomplete identities
// before opening or creating the owner operation ledger.
func ValidateCreateRequest(request CreateRequest) error {
	return validateCreateRequest(request)
}

func boundedJSON(value json.RawMessage, limit int) ([]byte, error) {
	if len(value) == 0 {
		return nil, nil
	}
	if len(value) > limit {
		return nil, operationError(CodePayloadTooLarge, "Operation result or receipt exceeds the bounded ledger limit", nil)
	}
	if !json.Valid(value) {
		return nil, operationError(CodePayloadInvalid, "Operation result or receipt is invalid JSON", nil)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return nil, operationError(CodePayloadInvalid, "Operation result or receipt is invalid JSON", err)
	}
	if compact.Len() > limit {
		return nil, operationError(CodePayloadTooLarge, "Operation result or receipt exceeds the bounded ledger limit", nil)
	}
	return compact.Bytes(), nil
}

func validateOutcome(outcome Outcome) error {
	checks := []struct {
		value string
		limit int
	}{
		{outcome.ReceiptRef, 512},
		{outcome.ResourceRef, 512},
		{outcome.RevisionAfter, 160},
		{outcome.ErrorCode, 128},
		{outcome.ErrorMessage, 512},
	}
	for _, check := range checks {
		if err := validateLedgerRef(check.value, check.limit); err != nil {
			return err
		}
	}
	return nil
}

func validateLedgerRef(value string, limit int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > limit {
		return operationError(CodePayloadTooLarge, "Operation reference exceeds the bounded ledger limit", nil)
	}
	if strings.ContainsAny(value, "\x00\r\n") || filepath.IsAbs(value) || windowsAbsPattern.MatchString(value) || absPathFragmentPattern.MatchString(value) || strings.Contains(value, `\\`) {
		return operationError(CodePayloadInvalid, "Operation reference is not safe for the ledger", nil)
	}
	return nil
}

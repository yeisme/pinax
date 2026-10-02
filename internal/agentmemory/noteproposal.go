package agentmemory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

// NoteProposalRow extends the existing store without changing memory rows.
// Payload contains private user content and is excluded from all projections.
type NoteProposalRow struct {
	ProposalID       string    `gorm:"primaryKey" json:"proposal_id"`
	PrincipalID      string    `json:"principal_id"`
	ScopeKind        string    `json:"scope_kind"`
	ScopeID          string    `json:"scope_id"`
	Kind             string    `json:"kind"`
	Risk             string    `json:"risk"`
	Status           string    `gorm:"index" json:"status"`
	TargetObjectID   string    `json:"target_object_id,omitempty"`
	ExpectedRevision string    `json:"expected_revision,omitempty"`
	Rationale        string    `json:"rationale"`
	AddedLines       int       `json:"added_lines"`
	RemovedLines     int       `json:"removed_lines"`
	Hunks            int       `json:"hunks"`
	Payload          string    `json:"-"`
	ReceiptID        string    `json:"receipt_id,omitempty"`
	SnapshotID       string    `json:"snapshot_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	StaleCount       int       `json:"stale_count"`
}

func (NoteProposalRow) TableName() string { return "agent_note_proposals" }

// OpenNoteProposals opens the private writable payload store. Public reads use
// service-authored projections and do not open SQLite.
func OpenNoteProposals(root string) (*Store, error) {
	path := filepath.Join(root, ".pinax", "memory", "agent_memory.sqlite")
	s, err := Open(root)
	if err != nil {
		return nil, err
	}
	if err := s.db.AutoMigrate(&NoteProposalRow{}); err != nil {
		_ = s.Close()
		return nil, err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		_ = s.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) SaveNoteProposal(ctx context.Context, row *NoteProposalRow) error {
	return s.db.WithContext(ctx).Create(row).Error
}
func (s *Store) GetNoteProposal(ctx context.Context, id string) (NoteProposalRow, error) {
	var row NoteProposalRow
	err := s.db.WithContext(ctx).First(&row, "proposal_id = ?", id).Error
	return row, err
}
func (s *Store) ClaimNoteProposal(ctx context.Context, id, from, to string) (bool, error) {
	r := s.db.WithContext(ctx).Model(&NoteProposalRow{}).Where("proposal_id = ? AND status = ?", id, from).Update("status", to)
	return r.RowsAffected == 1, r.Error
}
func (s *Store) UpdateNoteProposal(ctx context.Context, row *NoteProposalRow, publish ...func(NoteProposalRow) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		from := []string{"applying"}
		if row.Status == "reconcile_required" {
			from = []string{"applying", "rejecting"}
		}
		if row.Status == "rejected" {
			from = []string{"rejecting"}
		}
		if row.Status == "approval_required" {
			from = []string{"approval_required"}
		}
		r := tx.Model(&NoteProposalRow{}).Where("proposal_id = ? AND status IN ?", row.ProposalID, from).Select("*").Updates(row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return fmt.Errorf("proposal_consumed: refresh the proposal before acting")
		}
		for _, emit := range publish {
			if err := emit(*row); err != nil {
				return err
			}
		}
		return nil
	})
}

// RefreshNoteProjection serializes a public projection refresh with mutations.
func (s *Store) RefreshNoteProjection(ctx context.Context, id string, publish func(NoteProposalRow) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&NoteProposalRow{}).Where("proposal_id = ?", id).UpdateColumn("proposal_id", id).Error; err != nil {
			return err
		}
		var row NoteProposalRow
		if err := tx.First(&row, "proposal_id = ?", id).Error; err != nil {
			return err
		}
		return publish(row)
	})
}

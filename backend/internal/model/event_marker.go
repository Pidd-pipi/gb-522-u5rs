package model

import (
	"time"

	"fiber-otdr-fault-localization/backend/internal/constants"
	"gorm.io/datatypes"
)

type EventMarker struct {
	ID                       uint                `gorm:"primaryKey" json:"id"`
	TraceID                  uint                `gorm:"not null;index" json:"trace_id"`
	DistanceM                float64             `gorm:"not null" json:"distance_m"`
	EventType                constants.EventType `gorm:"size:24;not null;index;check:event_type_allowed,event_type IN ('connector','splice','bend','break','end','unknown')" json:"event_type"`
	InsertionLossDB          float64             `gorm:"not null" json:"insertion_loss_db"`
	ReflectanceDB            float64             `gorm:"not null" json:"reflectance_db"`
	Confidence               float64             `gorm:"not null" json:"confidence"`
	AlgorithmEventType       constants.EventType `gorm:"size:24;not null;check:algorithm_event_type_allowed,algorithm_event_type IN ('connector','splice','bend','break','end','unknown')" json:"algorithm_event_type"`
	AlgorithmDistanceM       float64             `gorm:"not null" json:"algorithm_distance_m"`
	AlgorithmInsertionLossDB float64             `gorm:"not null" json:"algorithm_insertion_loss_db"`
	Reviewed                 bool                `gorm:"not null;default:false;index" json:"reviewed"`
	ReviewNote               string              `gorm:"size:1000" json:"review_note"`
	ReviewedBy               *uint               `json:"reviewed_by"`
	ReviewedAt               *time.Time          `json:"reviewed_at"`
	Version                  uint                `gorm:"not null;default:1" json:"version"`
	RevisionCount            int                 `gorm:"not null;default:0" json:"revision_count"`
	LastRevisionAt           *time.Time          `json:"last_revision_at"`
	LastRevisionJSON         datatypes.JSON      `gorm:"type:jsonb" json:"last_revision"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`
}

// EventRevision is an immutable entry in the review trail. Each successful
// review copies the previous human decision (nil on the first review) before
// the new decision overwrites EventMarker, so every correction stays traceable.
type EventRevision struct {
	ID             uint                 `gorm:"primaryKey" json:"id"`
	EventID        uint                 `gorm:"not null;index" json:"event_id"`
	RevisionNo     int                  `gorm:"not null" json:"revision_no"`
	EventType      constants.EventType  `gorm:"size:24;not null;check:revision_event_type_allowed,event_type IN ('connector','splice','bend','break','end','unknown')" json:"event_type"`
	DistanceM      float64              `gorm:"not null" json:"distance_m"`
	ReviewNote     string               `gorm:"size:1000;not null" json:"review_note"`
	ReviewedBy     uint                 `gorm:"not null" json:"reviewed_by"`
	ReviewerName   string               `gorm:"size:60;not null" json:"reviewer_name"`
	ReviewedAt     time.Time            `gorm:"not null" json:"reviewed_at"`
	PrevEventType  *constants.EventType `gorm:"size:24;check:revision_prev_event_type_allowed,prev_event_type IN ('connector','splice','bend','break','end','unknown')" json:"prev_event_type"`
	PrevDistanceM  *float64             `json:"prev_distance_m"`
	PrevReviewNote string               `gorm:"size:1000" json:"prev_review_note"`
	PrevReviewedBy *uint                `json:"prev_reviewed_by"`
	PrevReviewer   string               `gorm:"size:60" json:"prev_reviewer_name"`
	PrevReviewedAt *time.Time           `json:"prev_reviewed_at"`
	CreatedAt      time.Time            `json:"created_at"`
}

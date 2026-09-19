package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"fiber-otdr-fault-localization/backend/internal/algorithm"
	"fiber-otdr-fault-localization/backend/internal/constants"
	"fiber-otdr-fault-localization/backend/internal/dto"
	"fiber-otdr-fault-localization/backend/internal/model"
	"fiber-otdr-fault-localization/backend/internal/repository"
	"gorm.io/datatypes"
)

type EventService struct{ store *repository.Store }

func NewEventService(store *repository.Store) *EventService { return &EventService{store} }

func (s *EventService) Detect(traceID uint, request dto.DetectEventsRequest, actor Actor) (dto.DetectionSummary, error) {
	trace, err := s.store.Traces.Get(traceID)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.DetectionSummary{}, notFound("trace")
	}
	if err != nil {
		return dto.DetectionSummary{}, internal("get trace failed", err)
	}
	route, err := s.store.Routes.Get(trace.RouteID)
	if err != nil {
		return dto.DetectionSummary{}, internal("get trace route failed", err)
	}
	var raw []float64
	if err := json.Unmarshal(trace.RawPointsJSON, &raw); err != nil {
		return dto.DetectionSummary{}, internal("decode raw trace failed", err)
	}
	window := request.DenoiseWindow
	if window == 0 {
		window = trace.DenoiseWindow
	}
	if window == 0 {
		window = 5
	}
	threshold := request.PeakThresholdDB
	if threshold == 0 {
		threshold = trace.PeakThresholdDB
	}
	if threshold == 0 {
		threshold = 0.8
	}
	merge := request.MergeWindow
	if merge == 0 {
		merge = trace.MergeWindow
	}
	if merge == 0 {
		merge = 3
	}
	filtered, err := algorithm.MovingMedian(raw, window)
	if err != nil {
		return dto.DetectionSummary{}, &AppError{CodeAlgorithmInput, 422, "trace denoising failed", err}
	}
	noise, err := algorithm.EstimateNoiseFloor(filtered)
	if err != nil {
		return dto.DetectionSummary{}, &AppError{CodeAlgorithmInput, 422, "noise floor estimation failed", err}
	}
	detected, rejected, err := algorithm.Detect(filtered, threshold, merge, trace.SampleIntervalNS, route.RefractiveIndex, route.LengthM)
	if err != nil {
		return dto.DetectionSummary{}, &AppError{CodeAlgorithmInput, 422, "event detection failed", err}
	}
	events := make([]model.EventMarker, 0, len(detected))
	for _, item := range detected {
		events = append(events, model.EventMarker{TraceID: trace.ID, DistanceM: item.DistanceM, EventType: item.Type, InsertionLossDB: item.InsertionLossDB, ReflectanceDB: item.ReflectanceDB, Confidence: item.Confidence, AlgorithmEventType: item.Type, AlgorithmDistanceM: item.DistanceM, AlgorithmInsertionLossDB: item.InsertionLossDB})
	}
	processed, _ := json.Marshal(filtered)
	err = s.store.Transaction(func(tx *repository.Store) error {
		if err := tx.Traces.UpdateProcessing(trace.ID, datatypes.JSON(processed), noise, window, threshold, merge); err != nil {
			return err
		}
		if err := tx.Events.ReplaceForTrace(trace.ID, events); err != nil {
			return err
		}
		params := map[string]any{"denoise_window": window, "peak_threshold_db": threshold, "merge_window": merge, "noise_floor_db": noise, "detected": len(events), "rejected_out_of_bounds": rejected}
		return tx.Audits.Create(audit(actor, "trace.events_detected", "TraceCapture", trace.ID, &route.ID, "{}", snapshot(params)))
	})
	if err != nil {
		return dto.DetectionSummary{}, internal("save detected events failed", err)
	}
	return dto.DetectionSummary{TraceID: trace.ID, DetectedCount: len(events), NoiseFloorDB: noise, ThresholdDB: threshold, RejectedCount: rejected}, nil
}

func (s *EventService) List(query dto.EventQuery) ([]model.EventMarker, dto.Pagination, error) {
	normalizePage(&query.Page, &query.PageSize)
	items, total, err := s.store.Events.List(query)
	if err != nil {
		return nil, dto.Pagination{}, internal("list events failed", err)
	}
	return items, dto.Pagination{Page: query.Page, PageSize: query.PageSize, Total: total}, nil
}

func (s *EventService) Revisions(eventID uint) ([]dto.EventRevisionView, error) {
	if _, err := s.store.Events.Get(eventID); errors.Is(err, repository.ErrNotFound) {
		return nil, notFound("event")
	} else if err != nil {
		return nil, internal("get event failed", err)
	}
	items, err := s.store.Events.ListRevisions(eventID)
	if err != nil {
		return nil, internal("list event revisions failed", err)
	}
	views := make([]dto.EventRevisionView, 0, len(items))
	for _, item := range items {
		views = append(views, toRevisionView(item))
	}
	return views, nil
}

func (s *EventService) Review(id uint, request dto.ReviewEventRequest, actor Actor) (dto.ReviewEventResult, error) {
	if !request.EventType.Valid() {
		return dto.ReviewEventResult{}, invalid("event_type is not supported", nil)
	}
	event, err := s.store.Events.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		return dto.ReviewEventResult{}, notFound("event")
	}
	if err != nil {
		return dto.ReviewEventResult{}, internal("get event failed", err)
	}
	trace, err := s.store.Traces.Get(event.TraceID)
	if err != nil {
		return dto.ReviewEventResult{}, internal("get event trace failed", err)
	}
	route, err := s.store.Routes.Get(trace.RouteID)
	if err != nil {
		return dto.ReviewEventResult{}, internal("get event route failed", err)
	}
	// Optimistic concurrency: a stale dialog must not overwrite a newer decision.
	if request.Version != event.Version {
		return dto.ReviewEventResult{}, conflict(fmt.Sprintf("event revision %d is out of date; current version is %d", request.Version, event.Version), nil)
	}
	newDistance := event.DistanceM
	if request.DistanceM != nil {
		newDistance = *request.DistanceM
	}
	if newDistance > route.LengthM {
		return dto.ReviewEventResult{}, invalid("reviewed distance exceeds route length", nil)
	}

	var (
		updated            model.EventMarker
		revisionEntry      model.EventRevision
		invalidated        []model.LocalizationCase
		invalidationReason string
	)
	err = s.store.Transaction(func(tx *repository.Store) error {
		// A closed localization case is an immutable conclusion built on these
		// events; reject the revision instead of silently invalidating it.
		referencing, err := tx.Cases.ReferencingTrace(event.TraceID)
		if err != nil {
			return err
		}
		for _, item := range referencing {
			if item.CaseStatus == constants.CaseClosed {
				return &AppError{CodeConflict, 409, fmt.Sprintf("event is locked by closed case #%d and cannot be revised", item.ID), nil}
			}
		}

		before := event
		now := time.Now()
		event.EventType = request.EventType
		event.DistanceM = newDistance
		event.Reviewed = true
		event.ReviewNote = request.ReviewNote
		event.ReviewedBy = &actor.ID
		event.ReviewedAt = &now
		event.RevisionCount = before.RevisionCount + 1

		revisionEntry = buildRevision(before, event, actor, now)
		if err := tx.Events.CreateRevision(&revisionEntry); err != nil {
			return err
		}
		summary, _ := json.Marshal(toRevisionView(revisionEntry))
		event.LastRevisionAt = &now
		event.LastRevisionJSON = datatypes.JSON(summary)

		applied, err := tx.Events.Review(&event)
		if err != nil {
			return err
		}
		if !applied {
			// Lost the race against a concurrent revision.
			return &AppError{CodeConflict, 409, "event was revised concurrently; reload and retry", nil}
		}
		if err := tx.Audits.Create(audit(actor, "event.reviewed", "EventMarker", event.ID, &route.ID, snapshot(before), snapshot(event))); err != nil {
			return err
		}

		reason := fmt.Sprintf("referenced event #%d was revised by %s at %s; previous analysis is stale", event.ID, actor.Username, now.Format(time.RFC3339))
		invalidationReason = reason
		invalidated, err = tx.Cases.InvalidateForEventRevision(event.TraceID, reason)
		if err != nil {
			return err
		}
		for _, item := range invalidated {
			if err := tx.Audits.Create(audit(actor, "case.invalidated_by_event_revision", "LocalizationCase", item.ID, &route.ID, snapshot(map[string]any{"status": constants.CaseDraft, "requires_reanalysis": false}), snapshot(map[string]any{"status": constants.CaseDraft, "requires_reanalysis": true, "reason": reason, "event_id": event.ID}))); err != nil {
				return err
			}
		}

		updated, err = tx.Events.Get(event.ID)
		return err
	})
	if err != nil {
		var appErr *AppError
		if errors.As(err, &appErr) {
			return dto.ReviewEventResult{}, appErr
		}
		return dto.ReviewEventResult{}, internal("review event failed", err)
	}

	cases := make([]dto.CaseInvalidation, 0, len(invalidated))
	for _, item := range invalidated {
		cases = append(cases, dto.CaseInvalidation{CaseID: item.ID, Reason: invalidationReason, Status: string(item.CaseStatus), Version: item.Version})
	}
	return dto.ReviewEventResult{Event: updated, Revision: toRevisionView(revisionEntry), InvalidatedCases: cases}, nil
}

// buildRevision copies the previous human decision before it is overwritten.
// For a first-ever review the previous pointers stay nil.
func buildRevision(before model.EventMarker, after model.EventMarker, actor Actor, at time.Time) model.EventRevision {
	entry := model.EventRevision{
		EventID:      after.ID,
		RevisionNo:   after.RevisionCount,
		EventType:    after.EventType,
		DistanceM:    after.DistanceM,
		ReviewNote:   after.ReviewNote,
		ReviewedBy:   actor.ID,
		ReviewerName: actor.Username,
		ReviewedAt:   at,
	}
	if before.Reviewed && before.ReviewedBy != nil && before.ReviewedAt != nil {
		prevType := before.EventType
		prevDistance := before.DistanceM
		prevBy := *before.ReviewedBy
		prevAt := *before.ReviewedAt
		entry.PrevEventType = &prevType
		entry.PrevDistanceM = &prevDistance
		entry.PrevReviewNote = before.ReviewNote
		entry.PrevReviewedBy = &prevBy
		entry.PrevReviewedAt = &prevAt
	}
	return entry
}

func toRevisionView(item model.EventRevision) dto.EventRevisionView {
	return dto.EventRevisionView{
		ID:             item.ID,
		RevisionNo:     item.RevisionNo,
		EventType:      item.EventType,
		DistanceM:      item.DistanceM,
		ReviewNote:     item.ReviewNote,
		ReviewedBy:     item.ReviewedBy,
		ReviewerName:   item.ReviewerName,
		ReviewedAt:     item.ReviewedAt,
		PrevEventType:  item.PrevEventType,
		PrevDistanceM:  item.PrevDistanceM,
		PrevReviewNote: item.PrevReviewNote,
		PrevReviewedBy: item.PrevReviewedBy,
		PrevReviewer:   item.PrevReviewer,
		PrevReviewedAt: item.PrevReviewedAt,
	}
}

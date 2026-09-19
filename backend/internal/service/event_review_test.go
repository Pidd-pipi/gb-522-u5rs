package service

import (
	"errors"
	"testing"
	"time"

	"fiber-otdr-fault-localization/backend/internal/constants"
	"fiber-otdr-fault-localization/backend/internal/dto"
	"fiber-otdr-fault-localization/backend/internal/model"
	"fiber-otdr-fault-localization/backend/internal/repository"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newReviewFixture(t *testing.T) (*EventService, *repository.Store, model.FiberRoute, model.TraceCapture) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.FiberRoute{}, &model.TraceCapture{}, &model.EventMarker{}, &model.EventRevision{}, &model.LocalizationCase{}, &model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	store := repository.NewStore(db)
	route := model.FiberRoute{RouteCode: "RT-" + t.Name(), Name: "revision route", LengthM: 50000, RefractiveIndex: 1.4682, LaunchConnector: "SC/APC", RouteStatus: "active"}
	if err := db.Create(&route).Error; err != nil {
		t.Fatal(err)
	}
	trace := model.TraceCapture{RouteID: route.ID, WavelengthNM: 1550, PulseWidthNS: 100, SampleIntervalNS: 40, RawPointsJSON: datatypes.JSON([]byte("[0.1,0.2]")), CapturedAt: time.Now(), UploadedBy: 1}
	if err := db.Create(&trace).Error; err != nil {
		t.Fatal(err)
	}
	return NewEventService(store), store, route, trace
}

func reviewActor() Actor {
	return Actor{ID: 7, Username: "reviewer", Role: constants.RoleReviewer, RequestID: "req-test"}
}

func reviewedEvent(t *testing.T, store *repository.Store, traceID uint) model.EventMarker {
	t.Helper()
	by := uint(3)
	at := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	event := model.EventMarker{
		TraceID: traceID, DistanceM: 1200, EventType: constants.EventSplice, InsertionLossDB: 0.3, ReflectanceDB: -50, Confidence: 0.9,
		AlgorithmEventType: constants.EventSplice, AlgorithmDistanceM: 1200, AlgorithmInsertionLossDB: 0.3,
		Reviewed: true, ReviewNote: "initial judgment", ReviewedBy: &by, ReviewedAt: &at,
	}
	if err := store.DB.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	return event
}

func TestReviewArchivesSupersededJudgment(t *testing.T) {
	service, store, _, trace := newReviewFixture(t)
	event := reviewedEvent(t, store, trace.ID)
	distance := 1250.5
	updated, err := service.Review(event.ID, dto.ReviewEventRequest{EventType: constants.EventConnector, DistanceM: &distance, ReviewNote: "corrected to connector", Version: event.Version}, reviewActor())
	if err != nil {
		t.Fatalf("re-review failed: %v", err)
	}
	if updated.RevisionCount != 1 || updated.Version != event.Version+1 || updated.EventType != constants.EventConnector {
		t.Fatalf("unexpected event after revision: %+v", updated)
	}
	revisions, err := service.Revisions(event.ID)
	if err != nil {
		t.Fatalf("list revisions failed: %v", err)
	}
	if len(revisions) != 1 {
		t.Fatalf("expected one archived revision, got %d", len(revisions))
	}
	archived := revisions[0]
	if archived.PreviousEventType != constants.EventSplice || archived.PreviousDistanceM != 1200 || archived.PreviousReviewNote != "initial judgment" {
		t.Fatalf("previous judgment not preserved: %+v", archived)
	}
	if archived.PreviousReviewedAt == nil || !archived.PreviousReviewedAt.Equal(*event.ReviewedAt) {
		t.Fatalf("previous review timestamp not preserved: %+v", archived.PreviousReviewedAt)
	}
	if archived.NewEventType != constants.EventConnector || archived.NewDistanceM != distance || archived.RevisionNo != 1 {
		t.Fatalf("new judgment not recorded: %+v", archived)
	}
	var audits []model.AuditLog
	if err := store.DB.Where("action = ? AND resource_id = ?", "event.revised", event.ID).Find(&audits).Error; err != nil || len(audits) != 1 {
		t.Fatalf("expected one event.revised audit, got %d err=%v", len(audits), err)
	}
}

func TestReviewAdmitsOnlyOneConcurrentRevision(t *testing.T) {
	service, store, _, trace := newReviewFixture(t)
	event := reviewedEvent(t, store, trace.ID)
	request := dto.ReviewEventRequest{EventType: constants.EventBend, ReviewNote: "first concurrent attempt", Version: event.Version}
	if _, err := service.Review(event.ID, request, reviewActor()); err != nil {
		t.Fatalf("first revision should win: %v", err)
	}
	stale := dto.ReviewEventRequest{EventType: constants.EventBreak, ReviewNote: "second concurrent attempt", Version: event.Version}
	_, err := service.Review(event.ID, stale, reviewActor())
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict {
		t.Fatalf("stale revision must conflict, got %v", err)
	}
	reloaded, getErr := store.Events.Get(event.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if reloaded.EventType != constants.EventBend || reloaded.RevisionCount != 1 {
		t.Fatalf("losing revision must not overwrite winner: %+v", reloaded)
	}
}

func TestReviewInvalidatesOpenCases(t *testing.T) {
	service, store, route, trace := newReviewFixture(t)
	event := reviewedEvent(t, store, trace.ID)
	open := model.LocalizationCase{RouteID: route.ID, BaselineTraceID: trace.ID, CurrentTraceID: trace.ID + 999, CaseStatus: constants.CasePendingReview, ParametersJSON: datatypes.JSON([]byte(`{}`)), DifferencesJSON: datatypes.JSON([]byte(`[]`)), Version: 4, CreatedBy: 1}
	if err := store.DB.Create(&open).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Review(event.ID, dto.ReviewEventRequest{EventType: constants.EventBend, ReviewNote: "bend not splice", Version: event.Version}, reviewActor()); err != nil {
		t.Fatalf("review failed: %v", err)
	}
	var reloaded model.LocalizationCase
	if err := store.DB.First(&reloaded, open.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.CaseStatus != constants.CaseDraft || reloaded.InvalidationReason == "" || reloaded.Version != 5 {
		t.Fatalf("open case must return to draft with reason: %+v", reloaded)
	}
	var audits []model.AuditLog
	if err := store.DB.Where("action = ? AND resource_id = ?", "case.invalidated", open.ID).Find(&audits).Error; err != nil || len(audits) != 1 {
		t.Fatalf("expected one case.invalidated audit, got %d err=%v", len(audits), err)
	}
}

func TestReviewRejectedWhenClosedCaseReferencesTrace(t *testing.T) {
	service, store, route, trace := newReviewFixture(t)
	event := reviewedEvent(t, store, trace.ID)
	closed := model.LocalizationCase{RouteID: route.ID, BaselineTraceID: trace.ID, CurrentTraceID: trace.ID + 999, CaseStatus: constants.CaseClosed, ParametersJSON: datatypes.JSON([]byte(`{}`)), DifferencesJSON: datatypes.JSON([]byte(`[]`)), Version: 6, CreatedBy: 1}
	if err := store.DB.Create(&closed).Error; err != nil {
		t.Fatal(err)
	}
	_, err := service.Review(event.ID, dto.ReviewEventRequest{EventType: constants.EventBend, ReviewNote: "must be rejected", Version: event.Version}, reviewActor())
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeConflict {
		t.Fatalf("closed case reference must reject revision, got %v", err)
	}
	reloaded, getErr := store.Events.Get(event.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if reloaded.EventType != constants.EventSplice || reloaded.RevisionCount != 0 {
		t.Fatalf("rejected revision must leave event untouched: %+v", reloaded)
	}
}

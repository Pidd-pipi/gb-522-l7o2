package service

import (
	"errors"
	"testing"
	"time"

	"fiber-otdr-fault-localization/backend/internal/algorithm"
	"fiber-otdr-fault-localization/backend/internal/constants"
	"fiber-otdr-fault-localization/backend/internal/dto"
	"fiber-otdr-fault-localization/backend/internal/model"
	"fiber-otdr-fault-localization/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestStore(t *testing.T) *repository.Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.FiberRoute{}, &model.TraceCapture{}, &model.EventMarker{}, &model.LocalizationCase{}, &model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	return repository.NewStore(db)
}

func stepPoints(n int, drops map[int]float64) []float64 {
	points := make([]float64, n)
	offset := 0.0
	for i := range points {
		if drop, ok := drops[i]; ok {
			offset -= drop
		}
		points[i] = 10 - float64(i)*0.01 + offset
	}
	return points
}

func createTestRoute(t *testing.T, store *repository.Store, length, offset float64) model.FiberRoute {
	t.Helper()
	route := model.FiberRoute{RouteCode: "R-" + t.Name(), Name: "测试线路", LengthM: length, RefractiveIndex: 1.468, LaunchOffsetM: offset, LaunchConnector: "SC/APC", RouteStatus: "active"}
	if err := store.Routes.Create(&route); err != nil {
		t.Fatal(err)
	}
	return route
}

func importTestTrace(t *testing.T, traceService *TraceService, routeID uint, points []float64, actor Actor) model.TraceCapture {
	t.Helper()
	trace, err := traceService.Import(dto.ImportTraceRequest{RouteID: routeID, WavelengthNM: 1550, PulseWidthNS: 100, SampleIntervalNS: 100, Points: points, CapturedAt: time.Now()}, actor)
	if err != nil {
		t.Fatalf("import trace: %v", err)
	}
	return trace
}

func TestImportFreezesRouteSnapshot(t *testing.T) {
	store := newTestStore(t)
	routeService := NewRouteService(store)
	traceService := NewTraceService(store, 20000)
	admin := Actor{ID: 1, Username: "admin", Role: constants.RoleAdmin, RequestID: "test-request"}
	route, err := routeService.Create(dto.CreateRouteRequest{RouteCode: "SNAP01", Name: "快照线路", LengthM: 5000, RefractiveIndex: 1.468, LaunchOffsetM: 200, LaunchConnector: "SC/APC"}, admin)
	if err != nil {
		t.Fatal(err)
	}
	trace := importTestTrace(t, traceService, route.ID, stepPoints(120, map[int]float64{40: 8}), admin)
	if trace.RouteLengthSnapshotM == nil || *trace.RouteLengthSnapshotM != 5000 {
		t.Fatalf("length snapshot = %v, want 5000", trace.RouteLengthSnapshotM)
	}
	if trace.RefractiveIndexSnapshot == nil || *trace.RefractiveIndexSnapshot != 1.468 {
		t.Fatalf("refractive snapshot = %v, want 1.468", trace.RefractiveIndexSnapshot)
	}
	if trace.LaunchOffsetSnapshotM == nil || *trace.LaunchOffsetSnapshotM != 200 {
		t.Fatalf("offset snapshot = %v, want 200", trace.LaunchOffsetSnapshotM)
	}
}

func TestDetectUsesFrozenSnapshot(t *testing.T) {
	store := newTestStore(t)
	routeService := NewRouteService(store)
	traceService := NewTraceService(store, 20000)
	eventService := NewEventService(store)
	admin := Actor{ID: 1, Username: "admin", Role: constants.RoleAdmin, RequestID: "test-request"}
	route := createTestRoute(t, store, 5000, 0)
	trace := importTestTrace(t, traceService, route.ID, stepPoints(120, map[int]float64{40: 8}), admin)
	if _, err := eventService.Detect(trace.ID, dto.DetectEventsRequest{PeakThresholdDB: 6}, admin); err != nil {
		t.Fatalf("first detect: %v", err)
	}
	events, err := store.Events.ForTrace(trace.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected one event, got %d (err=%v)", len(events), err)
	}
	firstDistance := events[0].DistanceM

	offset := 200.0
	if _, err := routeService.Update(route.ID, dto.UpdateRouteRequest{LaunchOffsetM: &offset}, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := eventService.Detect(trace.ID, dto.DetectEventsRequest{PeakThresholdDB: 6}, admin); err != nil {
		t.Fatalf("detect after route change: %v", err)
	}
	events, _ = store.Events.ForTrace(trace.ID)
	if len(events) != 1 || events[0].DistanceM != firstDistance {
		t.Fatalf("historical events must keep snapshot distance %.2f, got %#v", firstDistance, events)
	}

	fresh := importTestTrace(t, traceService, route.ID, stepPoints(120, map[int]float64{40: 8}), admin)
	if _, err := eventService.Detect(fresh.ID, dto.DetectEventsRequest{PeakThresholdDB: 6}, admin); err != nil {
		t.Fatalf("detect fresh trace: %v", err)
	}
	freshEvents, _ := store.Events.ForTrace(fresh.ID)
	if len(freshEvents) != 1 || freshEvents[0].DistanceM != firstDistance-offset {
		t.Fatalf("new trace should apply %.0f m offset: want %.2f, got %#v", offset, firstDistance-offset, freshEvents)
	}
}

func TestDetectMissingSnapshotTreatsOffsetAsZero(t *testing.T) {
	store := newTestStore(t)
	routeService := NewRouteService(store)
	traceService := NewTraceService(store, 20000)
	eventService := NewEventService(store)
	admin := Actor{ID: 1, Username: "admin", Role: constants.RoleAdmin, RequestID: "test-request"}
	route := createTestRoute(t, store, 5000, 0)
	trace := importTestTrace(t, traceService, route.ID, stepPoints(120, map[int]float64{40: 8}), admin)
	if err := store.DB.Model(&model.TraceCapture{}).Where("id = ?", trace.ID).
		Updates(map[string]any{"route_length_snapshot_m": nil, "refractive_index_snapshot": nil, "launch_offset_snapshot_m": nil}).Error; err != nil {
		t.Fatal(err)
	}
	offset := 300.0
	if _, err := routeService.Update(route.ID, dto.UpdateRouteRequest{LaunchOffsetM: &offset}, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := eventService.Detect(trace.ID, dto.DetectEventsRequest{PeakThresholdDB: 6}, admin); err != nil {
		t.Fatalf("legacy trace detect: %v", err)
	}
	events, _ := store.Events.ForTrace(trace.ID)
	want, _ := algorithm.RouteDistance(40, 100, 1.468, 0)
	if len(events) != 1 || events[0].DistanceM != want {
		t.Fatalf("legacy trace should use zero offset, want %.2f got %#v", want, events)
	}
}

func TestDetectBeyondRouteLengthRejectsAndKeepsOldResults(t *testing.T) {
	store := newTestStore(t)
	traceService := NewTraceService(store, 20000)
	eventService := NewEventService(store)
	admin := Actor{ID: 1, Username: "admin", Role: constants.RoleAdmin, RequestID: "test-request"}
	route := createTestRoute(t, store, 1000, 0)
	// 40 号样本约 408 m 在线路内；110 号样本约 1123 m 超出 1000 m。
	trace := importTestTrace(t, traceService, route.ID, stepPoints(120, map[int]float64{40: 8, 110: 5}), admin)
	if _, err := eventService.Detect(trace.ID, dto.DetectEventsRequest{PeakThresholdDB: 6}, admin); err != nil {
		t.Fatalf("first detect should keep the in-range event: %v", err)
	}
	events, _ := store.Events.ForTrace(trace.ID)
	if len(events) != 1 {
		t.Fatalf("expected one saved event, got %#v", events)
	}

	_, err := eventService.Detect(trace.ID, dto.DetectEventsRequest{DenoiseWindow: 11, PeakThresholdDB: 3}, admin)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeInvalidInput {
		t.Fatalf("expected INVALID_INPUT rejection, got %v", err)
	}
	events, _ = store.Events.ForTrace(trace.ID)
	if len(events) != 1 {
		t.Fatalf("old events must be preserved after rejection, got %#v", events)
	}
	reloaded, _ := store.Traces.Get(trace.ID)
	if reloaded.DenoiseWindow != 5 {
		t.Fatalf("old processing parameters must be preserved, window = %d", reloaded.DenoiseWindow)
	}
}

func TestReviewerCanOnlyUpdateLaunchOffset(t *testing.T) {
	store := newTestStore(t)
	routeService := NewRouteService(store)
	admin := Actor{ID: 1, Username: "admin", Role: constants.RoleAdmin, RequestID: "test-request"}
	reviewer := Actor{ID: 2, Username: "reviewer", Role: constants.RoleReviewer, RequestID: "test-request"}
	route := createTestRoute(t, store, 5000, 0)

	offset := 150.0
	updated, err := routeService.Update(route.ID, dto.UpdateRouteRequest{LaunchOffsetM: &offset}, reviewer)
	if err != nil {
		t.Fatalf("reviewer should update launch offset: %v", err)
	}
	if updated.LaunchOffsetM != 150 {
		t.Fatalf("launch offset = %.2f, want 150", updated.LaunchOffsetM)
	}
	newName := "不允许的改名"
	_, err = routeService.Update(route.ID, dto.UpdateRouteRequest{Name: &newName}, reviewer)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeForbidden {
		t.Fatalf("reviewer rename must be forbidden, got %v", err)
	}
	reloaded, _ := store.Routes.Get(route.ID)
	if reloaded.Name != route.Name {
		t.Fatalf("route name must remain unchanged, got %q", reloaded.Name)
	}

	length := 6000.0
	if _, err := routeService.Update(route.ID, dto.UpdateRouteRequest{LengthM: &length, LaunchOffsetM: &offset}, admin); err != nil {
		t.Fatalf("analyst/admin should keep full edit rights: %v", err)
	}
}

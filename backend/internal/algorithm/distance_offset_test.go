package algorithm

import (
	"testing"
)

func TestRouteDistanceAppliesLaunchOffset(t *testing.T) {
	tests := []struct {
		name    string
		index   int
		offset  float64
		want    float64
		wantErr bool
	}{
		{"zero offset keeps raw distance", 5, 0, 51.05, false},
		{"launch pigtail is subtracted", 5, 20, 31.05, false},
		{"event inside launch lead is negative", 1, 20, -9.79, false},
		{"negative offset extends beyond otdr origin", 5, -10, 61.05, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := RouteDistance(test.index, 100, 1.468, test.offset)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("route distance = %.2f, want %.2f", got, test.want)
			}
		})
	}
}

func TestDetectRejectsEventsBeyondRouteLength(t *testing.T) {
	// A steadily dropping trace with a sharp drop near the end.
	points := make([]float64, 120)
	for i := range points {
		points[i] = 30 - float64(i)*0.02
		if i >= 100 {
			points[i] -= 6
		}
	}
	// Raw span of 120 samples at 50 ns / 1.468 is about 613 m. A 500 m
	// launch offset leaves ~113 m of route, so the end drop at ~510 m raw
	// converts to ~10 m: valid for a 200 m route.
	events, _, err := Detect(points, 0.8, 3, 50, 1.468, 500, 200)
	if err != nil {
		t.Fatalf("expected detection to pass within route length: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event within the route")
	}
	for _, event := range events {
		if event.DistanceM < 0 || event.DistanceM > 200 {
			t.Fatalf("event distance %.2f outside route", event.DistanceM)
		}
	}

	// Same trace without the offset places the end event beyond 200 m: the
	// whole run must be rejected so callers retain their previous results.
	_, _, err = Detect(points, 0.8, 3, 50, 1.468, 0, 200)
	if err == nil {
		t.Fatal("expected an error when an event exceeds the route length")
	}
}

func TestDetectSkipsEventsInsideLaunchLead(t *testing.T) {
	points := make([]float64, 120)
	for i := range points {
		points[i] = 30 - float64(i)*0.01
		if i >= 10 {
			points[i] -= 5
		}
	}
	// Sample 10 at 50 ns / 1.468 is about 51 m raw; with a 100 m offset it
	// falls inside the launch lead and must be skipped, not rejected.
	events, rejected, err := Detect(points, 0.8, 3, 50, 1.468, 100, 2000)
	if err != nil {
		t.Fatalf("expected launch lead events to be skipped: %v", err)
	}
	if rejected != 1 {
		t.Fatalf("expected 1 skipped launch lead event, got %d", rejected)
	}
	if len(events) != 0 {
		t.Fatalf("expected no route events, got %d", len(events))
	}
}

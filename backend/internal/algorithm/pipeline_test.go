package algorithm

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

type traceFixture struct {
	Raw                    []float64 `json:"raw"`
	Window                 int       `json:"window"`
	Threshold              float64   `json:"threshold"`
	MergeWindow            int       `json:"merge_window"`
	SampleIntervalNS       float64   `json:"sample_interval_ns"`
	RefractiveIndex        float64   `json:"refractive_index"`
	LaunchOffsetM          float64   `json:"launch_offset_m"`
	RouteLengthM           float64   `json:"route_length_m"`
	ExpectedEventCount     int       `json:"expected_event_count"`
	ExpectedFirstDistanceM float64   `json:"expected_first_distance_m"`
}

func TestPipelineFixture(t *testing.T) {
	encoded, err := os.ReadFile("testdata/trace_fixture.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture traceFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	filtered, err := MovingMedian(fixture.Raw, fixture.Window)
	if err != nil {
		t.Fatalf("denoise fixture: %v", err)
	}
	events, skipped, err := Detect(filtered, fixture.Threshold, fixture.MergeWindow, fixture.SampleIntervalNS, fixture.RefractiveIndex, fixture.LaunchOffsetM, fixture.RouteLengthM)
	if err != nil {
		t.Fatalf("detect fixture: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("expected no pre-route events, got %d", skipped)
	}
	if len(events) != fixture.ExpectedEventCount {
		t.Fatalf("expected %d event, got %d: %#v", fixture.ExpectedEventCount, len(events), events)
	}
	if events[0].DistanceM != fixture.ExpectedFirstDistanceM {
		t.Fatalf("expected distance %.2f, got %.2f", fixture.ExpectedFirstDistanceM, events[0].DistanceM)
	}
}

func stepTrace(n int, drops map[int]float64) []float64 {
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

func TestDetectLaunchOffset(t *testing.T) {
	filtered, err := MovingMedian(stepTrace(200, map[int]float64{60: 5, 100: 5}), 5)
	if err != nil {
		t.Fatalf("denoise: %v", err)
	}
	const interval, refractive = 100.0, 1.468
	offset, _ := SampleDistance(60, interval, refractive)
	offset += 10
	events, skipped, err := Detect(filtered, 1, 3, interval, refractive, offset, 100000)
	if err != nil {
		t.Fatalf("detect with offset: %v", err)
	}
	if skipped != 1 {
		t.Fatalf("expected one pre-route peak skipped, got %d", skipped)
	}
	if len(events) != 1 {
		t.Fatalf("expected one in-route event, got %d: %#v", len(events), events)
	}
	expected, _ := RouteDistance(100, interval, refractive, offset)
	if events[0].DistanceM != expected {
		t.Fatalf("expected offset distance %.2f, got %.2f", expected, events[0].DistanceM)
	}
}

func TestDetectRejectsEventsBeyondRouteLength(t *testing.T) {
	filtered, err := MovingMedian(stepTrace(200, map[int]float64{150: 5}), 5)
	if err != nil {
		t.Fatalf("denoise: %v", err)
	}
	const interval, refractive, offset = 100.0, 1.468, 0.0
	events, _, err := Detect(filtered, 1, 3, interval, refractive, offset, 500)
	if err == nil {
		t.Fatalf("expected rejection beyond route length, got events %#v", events)
	}
	var outOfBounds *OutOfBoundsError
	if !errors.As(err, &outOfBounds) {
		t.Fatalf("expected *OutOfBoundsError, got %T: %v", err, err)
	}
	if outOfBounds.DistanceM <= 500 {
		t.Fatalf("rejected distance %.2f should exceed 500 m", outOfBounds.DistanceM)
	}
}

func TestSampleDistanceTable(t *testing.T) {
	tests := []struct {
		name                 string
		index                int
		interval, refractive float64
		launchOffset         float64
		want                 float64
		wantErr              bool
	}{
		{"origin", 0, 100, 1.468, 0, 0, false},
		{"five samples", 5, 100, 1.468, 0, 51.05, false},
		{"launch offset subtracts", 5, 100, 1.468, 20, 31.05, false},
		{"inside launch pigtail is negative", 1, 100, 1.468, 50, -39.79, false},
		{"negative index", -1, 100, 1.468, 0, 0, true},
		{"invalid refractive index", 2, 100, 2.0, 0, 0, true},
		{"negative launch offset", 2, 100, 1.468, -1, 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := RouteDistance(test.index, test.interval, test.refractive, test.launchOffset)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("distance = %.2f, want %.2f", got, test.want)
			}
		})
	}
}

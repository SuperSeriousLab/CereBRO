package main

import (
	"math"
	"testing"

	cerebrov1 "github.com/SuperSeriousLab/CereBRO/gen/go/cerebro/v1"
	"github.com/SuperSeriousLab/CereBRO/internal/pipeline"
)

func TestRun_HighUrgency(t *testing.T) {
	gain := &pipeline.GainSignal{Urgency: 0.9, Formality: 0.8, Complexity: 0.5, Mode: cerebrov1.GainMode_PHASIC}
	adj := Run(gain, DefaultConfig())

	for det, offset := range adj.Adjustments {
		if offset >= 0 {
			t.Errorf("expected negative offset for high urgency, got %.3f for %s", offset, det)
		}
	}
}

func TestRun_LowUrgency(t *testing.T) {
	gain := &pipeline.GainSignal{Urgency: 0.1, Formality: 0.2, Complexity: 0.1, Mode: cerebrov1.GainMode_TONIC}
	adj := Run(gain, DefaultConfig())

	for det, offset := range adj.Adjustments {
		if offset <= 0 {
			t.Errorf("expected positive offset for low urgency, got %.3f for %s", offset, det)
		}
	}
}

func TestRun_BoundsClamp(t *testing.T) {
	cfg := DefaultConfig()
	gain := &pipeline.GainSignal{Urgency: 1.0, Formality: 1.0, Complexity: 1.0}
	adj := Run(gain, cfg)

	for det, offset := range adj.Adjustments {
		if math.Abs(offset) > cfg.MaxGainOffset {
			t.Errorf("offset %.3f exceeds bounds for %s (max %.2f)", offset, det, cfg.MaxGainOffset)
		}
	}
}

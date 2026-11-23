package chaos

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ExperimentState tracks the progression of a Chaos Lab experiment.
type ExperimentState string

const (
	StatePending       ExperimentState = "PENDING"
	StateRunning       ExperimentState = "RUNNING"
	StateFaultInjected ExperimentState = "FAULT_INJECTED"
	StateRecovering    ExperimentState = "OBSERVING_RECOVERY"
	StateVerifying     ExperimentState = "VERIFYING_CONSISTENCY"
	StateCompleted     ExperimentState = "COMPLETED"
	StateFailed        ExperimentState = "FAILED"
)

// TimelineEntry records a timestamped operational event during a chaos run.
type TimelineEntry struct {
	Timestamp   time.Time      `json:"timestamp"`
	Phase       string         `json:"phase"`
	Description string         `json:"description"`
	Details     map[string]any `json:"details,omitempty"`
}

// ExperimentConfig defines the parameters for a chaos experiment.
type ExperimentConfig struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Target      string         `json:"target"`
	FaultType   string         `json:"fault_type"`
	Duration    time.Duration  `json:"duration_ns"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ExperimentReport aggregates findings, recovery timelines, and consistency verification.
type ExperimentReport struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Config          ExperimentConfig `json:"config"`
	State           ExperimentState `json:"state"`
	StartTime       time.Time       `json:"start_time"`
	EndTime         time.Time       `json:"end_time"`
	DurationSeconds float64         `json:"duration_seconds"`
	RecoveryTimeMs  float64         `json:"recovery_time_ms"`
	DataConsistent  bool            `json:"data_consistent"`
	InjectedFaults  []string        `json:"injected_faults"`
	Logs            []string        `json:"logs"`
	Timeline        []TimelineEntry `json:"timeline"`
	ErrorMsg        string          `json:"error_msg,omitempty"`
}

// ChaosLab orchestrates chaos experiments, fault injection timelines, and verification.
type ChaosLab struct {
	mu          sync.RWMutex
	experiments map[string]*ExperimentReport
	history     []*ExperimentReport
}

// NewChaosLab initializes an empty ChaosLab registry.
func NewChaosLab() *ChaosLab {
	return &ChaosLab{
		experiments: make(map[string]*ExperimentReport),
		history:     make([]*ExperimentReport, 0),
	}
}

// NewExperiment initializes a new experiment session in PENDING state.
func (lab *ChaosLab) NewExperiment(cfg ExperimentConfig) *ExperimentReport {
	lab.mu.Lock()
	defer lab.mu.Unlock()

	if cfg.ID == "" {
		cfg.ID = fmt.Sprintf("chaos-exp-%d", time.Now().UnixNano())
	}

	report := &ExperimentReport{
		ID:             cfg.ID,
		Name:           cfg.Name,
		Config:         cfg,
		State:          StatePending,
		StartTime:      time.Now().UTC(),
		InjectedFaults: make([]string, 0),
		Logs:           make([]string, 0),
		Timeline:       make([]TimelineEntry, 0),
		DataConsistent: true,
	}

	lab.experiments[cfg.ID] = report
	return report
}

// RecordTimeline appends a chronological milestone to the experiment.
func (lab *ChaosLab) RecordTimeline(id, phase, desc string, details map[string]any) {
	lab.mu.Lock()
	defer lab.mu.Unlock()

	rep, exists := lab.experiments[id]
	if !exists {
		return
	}
	entry := TimelineEntry{
		Timestamp:   time.Now().UTC(),
		Phase:       phase,
		Description: desc,
		Details:     details,
	}
	rep.Timeline = append(rep.Timeline, entry)
	rep.Logs = append(rep.Logs, fmt.Sprintf("[%s] %s: %s", entry.Timestamp.Format(time.RFC3339Nano), phase, desc))
}

// SetState transitions the experiment state.
func (lab *ChaosLab) SetState(id string, state ExperimentState) {
	lab.mu.Lock()
	defer lab.mu.Unlock()

	if rep, exists := lab.experiments[id]; exists {
		rep.State = state
	}
}

// CompleteExperiment marks the experiment as finished, records recovery duration and persists to history.
func (lab *ChaosLab) CompleteExperiment(id string, recoveryTime time.Duration, consistent bool, err error) *ExperimentReport {
	lab.mu.Lock()
	defer lab.mu.Unlock()

	rep, exists := lab.experiments[id]
	if !exists {
		return nil
	}

	rep.EndTime = time.Now().UTC()
	rep.DurationSeconds = rep.EndTime.Sub(rep.StartTime).Seconds()
	rep.RecoveryTimeMs = float64(recoveryTime.Microseconds()) / 1000.0
	rep.DataConsistent = consistent

	if err != nil {
		rep.State = StateFailed
		rep.ErrorMsg = err.Error()
	} else {
		rep.State = StateCompleted
	}

	lab.history = append(lab.history, rep)
	return rep
}

// GetExperiment retrieves an experiment report by ID.
func (lab *ChaosLab) GetExperiment(id string) (*ExperimentReport, bool) {
	lab.mu.RLock()
	defer lab.mu.RUnlock()
	rep, ok := lab.experiments[id]
	return rep, ok
}

// ListHistory returns all completed or failed experiments.
func (lab *ChaosLab) ListHistory() []*ExperimentReport {
	lab.mu.RLock()
	defer lab.mu.RUnlock()
	out := make([]*ExperimentReport, len(lab.history))
	copy(out, lab.history)
	return out
}

// ExportReportJSON saves an experiment report to a formatted JSON file.
func (lab *ChaosLab) ExportReportJSON(id, filePath string) error {
	rep, ok := lab.GetExperiment(id)
	if !ok {
		return fmt.Errorf("experiment %s not found", id)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// GenerateMarkdownReport produces an informative Markdown document for the chaos experiment.
func (lab *ChaosLab) GenerateMarkdownReport(id string) (string, error) {
	rep, ok := lab.GetExperiment(id)
	if !ok {
		return "", fmt.Errorf("experiment %s not found", id)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Chaos Experiment Report: %s\n\n", rep.Name))
	sb.WriteString(fmt.Sprintf("- **Experiment ID**: `%s`\n", rep.ID))
	sb.WriteString(fmt.Sprintf("- **Status**: `%s`\n", rep.State))
	sb.WriteString(fmt.Sprintf("- **Fault Type**: `%s`\n", rep.Config.FaultType))
	sb.WriteString(fmt.Sprintf("- **Target**: `%s`\n", rep.Config.Target))
	sb.WriteString(fmt.Sprintf("- **Recovery Time**: `%.2f ms`\n", rep.RecoveryTimeMs))
	sb.WriteString(fmt.Sprintf("- **Data Consistency Verified**: `%t`\n", rep.DataConsistent))
	sb.WriteString(fmt.Sprintf("- **Total Duration**: `%.2f s`\n\n", rep.DurationSeconds))

	if rep.ErrorMsg != "" {
		sb.WriteString(fmt.Sprintf("> ⚠️ **Error**: %s\n\n", rep.ErrorMsg))
	}

	sb.WriteString("### Event Timeline\n\n")
	sb.WriteString("| Time | Phase | Description |\n")
	sb.WriteString("| --- | --- | --- |\n")
	for _, entry := range rep.Timeline {
		sb.WriteString(fmt.Sprintf("| %s | `%s` | %s |\n", entry.Timestamp.Format("15:04:05.000"), entry.Phase, entry.Description))
	}

	return sb.String(), nil
}

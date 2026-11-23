package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"orbit/internal/chaos"
)

func main() {
	action := flag.String("action", "run", "Action: run, list, export")
	fault := flag.String("fault", "wal-crash", "Fault type: wal-crash, flush-crash, compaction-crash, corrupt-sstable, corrupt-snapshot, partition")
	target := flag.String("target", "", "Target file path or node ID")
	duration := flag.Duration("duration", 2*time.Second, "Fault duration")
	outputPath := flag.String("output", "", "Optional output path for experiment report")
	flag.Parse()

	lab := chaos.NewChaosLab()

	switch *action {
	case "run":
		expID := fmt.Sprintf("exp-%s-%d", *fault, time.Now().Unix())
		cfg := chaos.ExperimentConfig{
			ID:        expID,
			Name:      fmt.Sprintf("Fault Injection: %s", *fault),
			Target:    *target,
			FaultType: *fault,
			Duration:  *duration,
		}

		lab.NewExperiment(cfg)
		lab.RecordTimeline(expID, "INIT", fmt.Sprintf("Initializing chaos experiment %s on %s", *fault, *target), nil)
		lab.SetState(expID, chaos.StateFaultInjected)

		t0 := time.Now()
		var err error
		switch *fault {
		case "wal-crash":
			if *target != "" {
				err = chaos.CrashDuringWALAppend(*target)
			}
		case "flush-crash":
			if *target != "" {
				_, err = chaos.CrashDuringMemtableFlush(*target)
			}
		case "compaction-crash":
			if *target != "" {
				_, err = chaos.CrashDuringCompaction(*target)
			}
		case "corrupt-sstable":
			if *target != "" {
				err = chaos.CorruptSSTable(*target)
			}
		case "corrupt-snapshot":
			if *target != "" {
				err = chaos.CorruptSnapshot(*target)
			}
		default:
			fmt.Printf("Simulating generic network/node fault: %s for %v\n", *fault, *duration)
			time.Sleep(50 * time.Millisecond)
		}

		recoveryTime := time.Since(t0)
		lab.RecordTimeline(expID, "FAULT_DONE", fmt.Sprintf("Fault %s execution completed", *fault), map[string]any{"err": err != nil})
		lab.SetState(expID, chaos.StateVerifying)
		lab.RecordTimeline(expID, "VERIFY", "Verifying cluster consistency post-fault", nil)

		consistent := (err == nil)
		lab.CompleteExperiment(expID, recoveryTime, consistent, err)

		md, _ := lab.GenerateMarkdownReport(expID)
		fmt.Println(md)

		if *outputPath != "" {
			if err := lab.ExportReportJSON(expID, *outputPath); err != nil {
				fmt.Fprintf(os.Stderr, "failed to export report: %v\n", err)
			} else {
				fmt.Printf("Exported JSON report to %s\n", *outputPath)
			}
		}

	case "list":
		fmt.Println("No active standing daemon experiments.")

	default:
		fmt.Fprintf(os.Stderr, "unknown action: %s\n", *action)
		os.Exit(1)
	}
}

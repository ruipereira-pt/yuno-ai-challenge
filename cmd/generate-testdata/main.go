package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/testdata"
)

func main() {
	var (
		outPath  = flag.String("out", "testdata/transactions.json", "output file path")
		count    = flag.Int("count", 800, "number of events to generate")
		baseTime = flag.String("base-time", "2026-05-14T12:00:00Z", "base time in RFC3339 for deterministic generation")
	)
	flag.Parse()

	gen := testdata.NewGenerator()
	parsedBase, err := time.Parse(time.RFC3339, *baseTime)
	if err != nil {
		log.Fatalf("invalid -base-time (must be RFC3339): %v", err)
	}
	events := gen.GenerateTransactions(parsedBase.UTC(), *count)
	payload := model.BatchIngestRequest{Events: events}

	if err := os.MkdirAll("testdata", 0o750); err != nil {
		log.Fatalf("create output dir: %v", err)
	}
	file, err := os.Create(*outPath)
	if err != nil {
		log.Fatalf("create output file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		log.Fatalf("encode JSON: %v", err)
	}
	log.Printf("generated %d events to %s", len(events), *outPath)
}

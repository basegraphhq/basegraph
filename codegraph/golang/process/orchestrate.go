package process

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"basegraph.app/relay/common/arangodb"
	"basegraph.app/relay/common/typesense"
	"github.com/humanbeeng/lepo/prototypes/codegraph/extract"
)

// Orchestrate runs the code extraction and ingestion pipeline.
func Orchestrate(e extract.Extractor) {
	slog.Info("Begin orchestration")
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		slog.Info("Total time elapsed", "duration", humanizeDuration(elapsed))
	}()
	ctx := context.Background()

	// Step 1: Extract
	repoRoot := envOrDefault("TARGET_REPO_PATH", "/Users/nithin/basegraph/relay")
	targetModule := strings.TrimSpace(os.Getenv("TARGET_MODULE"))

	mods, err := discoverGoModules(repoRoot)
	if err != nil {
		slog.Error("discover go modules failed", "root", repoRoot, "err", err)
		return
	}

	if targetModule != "" {
		filtered := make([]goModule, 0, len(mods))
		for _, mod := range mods {
			if strings.Contains(mod.ModulePath, targetModule) {
				filtered = append(filtered, mod)
			}
		}
		if len(filtered) == 0 {
			slog.Warn("no modules matched TARGET_MODULE filter, skipping extraction", "module", targetModule)
			return
		}
		mods = filtered
	}

	slog.Info("Modules ready for extraction", "count", len(mods))

	acc := newExtractAccumulator()

	for _, mod := range mods {
		slog.Info("Extracting module", "module", mod.ModulePath, "dir", mod.Dir)
		moduleRes, extractErr := e.Extract(mod.ModulePath, mod.Dir)
		if extractErr != nil {
			slog.Error("module extraction failed", "module", mod.ModulePath, "dir", mod.Dir, "err", extractErr)
			return
		}
		mergeExtractResults(&acc, moduleRes)
	}

	extractRes := acc
	dataSize := float64(len(fmt.Sprintf("%+v", extractRes))) / (1024 * 1024)
	slog.Info("Extract result data size", "size_mb", dataSize)

	// Step 2: Create database clients
	tsClient, err := typesense.New(typesense.Config{
		URL:        envOrDefault("TYPESENSE_URL", "http://localhost:8108"),
		APIKey:     envOrDefault("TYPESENSE_API_KEY", "xyz"),
		Collection: envOrDefault("TYPESENSE_COLLECTION", "code_nodes"),
	})
	if err != nil {
		slog.Error("unable to create typesense client", "err", err)
		return
	}

	arangoClient, err := arangodb.New(ctx, arangodb.Config{
		URL:      envOrDefault("ARANGO_URL", "http://localhost:8529"),
		Username: envOrDefault("ARANGO_USERNAME", "root"),
		Password: envOrDefault("ARANGO_PASSWORD", ""),
		Database: envOrDefault("ARANGO_DATABASE", "codegraph"),
	})
	if err != nil {
		slog.Error("unable to create arangodb client", "err", err)
		return
	}
	defer func() {
		if closeErr := arangoClient.Close(); closeErr != nil {
			slog.Error("failed closing arangodb client", "err", closeErr)
		}
	}()

	// Step 3: Ingest into databases
	ingestor := NewIngestor(tsClient, arangoClient)
	slog.Info("Ingesting extract result into Typesense + ArangoDB")
	if err := ingestor.Ingest(ctx, extractRes); err != nil {
		slog.Error("ingestion failed", "err", err)
		return
	}
	slog.Info("Ingestion finished successfully")
}

func envOrDefault(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	if d < time.Millisecond {
		return d.String()
	}

	d = d.Round(time.Millisecond)
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second
	d -= seconds * time.Second
	milliseconds := d / time.Millisecond

	parts := make([]string, 0, 4)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	if milliseconds > 0 && hours == 0 && minutes == 0 && seconds == 0 {
		parts = append(parts, fmt.Sprintf("%dms", milliseconds))
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}

func ExportToCSV(extractRes extract.ExtractNodesResult) error {
	// Step 2: Export to CSV
	csvt := CSVNodeExporter{}
	var err error
	err = csvt.ExportTypes(extractRes.TypeDecls)
	if err != nil {
		return fmt.Errorf("unable to export types to csv: %w", err)
	}

	err = csvt.ExportMembers(extractRes.Members)
	if err != nil {
		return fmt.Errorf("unable to export members to csv: %w", err)
	}
	err = csvt.ExportInterfaces(extractRes.Interfaces)
	if err != nil {
		return fmt.Errorf("unable to export interfaces to csv: %w", err)
	}

	err = csvt.ExportNamed(extractRes.NamedTypes)
	if err != nil {
		return fmt.Errorf("unable to export named types to csv: %w", err)
	}

	err = csvt.ExportFile(extractRes.Files)
	if err != nil {
		return fmt.Errorf("unable to export files to csv: %w", err)
	}

	err = csvt.ExportFunctions(extractRes.Functions)
	if err != nil {
		return fmt.Errorf("unable to export calls to csv: %w", err)
	}

	err = csvt.ExportNamespace(extractRes.Namespaces)
	if err != nil {
		return fmt.Errorf("unable to export namespaces to csv: %w", err)
	}

	csvr := CSVRelationshipExporter{}
	//
	err = csvr.ExportCalls(extractRes.Functions)
	if err != nil {
		return fmt.Errorf("unable to export implements to csv: %w", err)
	}

	err = csvr.ExportImplements(extractRes.TypeDecls)
	if err != nil {
		return fmt.Errorf("unable to export implements to csv: %w", err)
	}

	err = csvr.ExportImports(extractRes.Files)
	if err != nil {
		return fmt.Errorf("unable to export imports to csv: %w", err)
	}

	err = csvr.ExportReturns(extractRes.Functions)
	if err != nil {
		return fmt.Errorf("unable to export returns to csv: %w", err)
	}

	err = csvr.ExportParams(extractRes.Functions)
	if err != nil {
		return fmt.Errorf("unable to export params to csv: %w", err)
	}

	slog.Info("Finished exporting to csv")
	return nil
}

func newExtractAccumulator() extract.ExtractNodesResult {
	return extract.ExtractNodesResult{
		TypeDecls:  make(map[string]extract.TypeDecl),
		Members:    make(map[string]extract.Member),
		Interfaces: make(map[string]extract.TypeDecl),
		Functions:  make(map[string]extract.Function),
		NamedTypes: make(map[string]extract.Named),
		Files:      make(map[string]extract.File),
		Namespaces: make([]extract.Namespace, 0),
		Vars:       make(map[string]extract.Variable),
	}
}

func mergeExtractResults(dst *extract.ExtractNodesResult, src extract.ExtractNodesResult) {
	for k, v := range src.TypeDecls {
		dst.TypeDecls[k] = v
	}
	for k, v := range src.Members {
		dst.Members[k] = v
	}
	for k, v := range src.Interfaces {
		dst.Interfaces[k] = v
	}
	for k, v := range src.Functions {
		dst.Functions[k] = v
	}
	for k, v := range src.NamedTypes {
		dst.NamedTypes[k] = v
	}
	for k, v := range src.Files {
		dst.Files[k] = v
	}
	for k, v := range src.Vars {
		dst.Vars[k] = v
	}
	dst.Namespaces = append(dst.Namespaces, src.Namespaces...)
}

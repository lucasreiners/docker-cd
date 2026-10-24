package desiredstate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lucasreiners/docker-cd/internal/operations"
)

const (
	WorkLayerLimit   = 200
	WorkImageLimit   = 32
	WorkOutcomeLimit = 100
	WorkDetailBudget = 2 * 1024 * 1024
	workByteInterval = 500 * time.Millisecond
	// Independently bounded identity census, not an omitted-detail shadow cache.
	// Once full, new layer observations are not collected. No raw IDs or bytes
	// survive here, and the census is released with its run.
	workCensusLimit = WorkDetailBudget / sha256.Size
)

type WorkOperation struct {
	Version         uint64     `json:"version"`
	ID              string     `json:"id"`
	ParentID        string     `json:"parentId,omitempty"`
	RootOperationID string     `json:"rootOperationId,omitempty"`
	Kind            string     `json:"kind"`
	Trigger         string     `json:"trigger"`
	Stack           string     `json:"stack,omitempty"`
	Revision        string     `json:"revision,omitempty"`
	Status          string     `json:"status"`
	Message         string     `json:"message"`
	StartedAt       time.Time  `json:"startedAt"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
}

type WorkLayer struct {
	ID           string    `json:"id"`
	Phase        string    `json:"phase"`
	Status       string    `json:"status"`
	BytesCurrent *int64    `json:"bytesCurrent,omitempty"`
	BytesTotal   *int64    `json:"bytesTotal,omitempty"`
	Done         bool      `json:"done"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type WorkImage struct {
	PullAttemptID    string      `json:"pullAttemptId"`
	Reference        string      `json:"reference"`
	Status           string      `json:"status"`
	Done             bool        `json:"done"`
	Position         int         `json:"position,omitempty"`
	Total            *int        `json:"total,omitempty"`
	LayerCount       int         `json:"layerCount"`
	Layers           []WorkLayer `json:"layers"`
	DetailsTruncated bool        `json:"detailsTruncated"`
}

type WorkStackOutcome struct {
	Stack       string `json:"stack"`
	Status      string `json:"status"`
	OperationID string `json:"operationId,omitempty"`
	Message     string `json:"message"`
}

type WorkStackResults struct {
	Processed        int                `json:"processed"`
	Skipped          int                `json:"skipped"`
	Failed           int                `json:"failed"`
	OutcomeCount     int                `json:"outcomeCount"`
	Outcomes         []WorkStackOutcome `json:"outcomes"`
	DetailsTruncated bool               `json:"detailsTruncated"`
}

type WorkRun struct {
	RunID             string            `json:"runId"`
	OperationID       string            `json:"operationId"`
	RootOperationID   string            `json:"rootOperationId"`
	ParentOperationID string            `json:"parentOperationId,omitempty"`
	Source            string            `json:"source"`
	Stack             string            `json:"stack,omitempty"`
	Revision          string            `json:"revision,omitempty"`
	CycleID           string            `json:"cycleId,omitempty"`
	Status            string            `json:"status"`
	Phase             string            `json:"phase"`
	StartedAt         time.Time         `json:"startedAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
	FinishedAt        *time.Time        `json:"finishedAt,omitempty"`
	Message           string            `json:"message"`
	ErrorCode         string            `json:"errorCode,omitempty"`
	ImageCount        int               `json:"imageCount"`
	Images            []WorkImage       `json:"images"`
	StackResults      *WorkStackResults `json:"stackResults,omitempty"`
	DetailsTruncated  bool              `json:"detailsTruncated"`
}

type WorkSnapshot struct {
	SchemaVersion    int             `json:"schemaVersion"`
	Epoch            string          `json:"epoch"`
	Sequence         uint64          `json:"sequence"`
	Operations       []WorkOperation `json:"operations"`
	Runs             []WorkRun       `json:"runs"`
	DetailsTruncated bool            `json:"detailsTruncated"`
}

type WorkUpsert struct {
	SchemaVersion int             `json:"schemaVersion"`
	Epoch         string          `json:"epoch"`
	Sequence      uint64          `json:"sequence"`
	Operations    []WorkOperation `json:"operations"`
	Run           *WorkRun        `json:"run,omitempty"`
}

type liveRun struct {
	WorkRun
	lastSent     time.Time
	timer        *time.Timer
	layersSeen   map[[sha256.Size]byte]struct{}
	imagesClosed bool
}

type workState struct {
	epoch         string
	sequence      uint64
	runs          map[string]*liveRun
	operations    map[string]WorkOperation
	censusEntries int
	detailBytes   int
}

func newWorkState() workState {
	return workState{epoch: uuid.NewString(), runs: make(map[string]*liveRun), operations: make(map[string]WorkOperation)}
}

// operationChainLocked reads history only while the publication barrier is held.
// Callers never hold a store lock while acquiring the broadcaster lock.
func (b *Broadcaster) operationChainLocked(id string) []WorkOperation {
	chain := []WorkOperation{}
	seen := make(map[string]bool)
	root := ""
	for id != "" && !seen[id] {
		seen[id] = true
		r, ok := b.operations.Get(id)
		if !ok {
			if cached, exists := b.work.operations[id]; exists {
				chain = append(chain, cached)
				if cached.ParentID == "" {
					root = cached.ID
				}
				id = cached.ParentID
				continue
			}
			break
		}
		// SDK/Compose errors in durable history are not safe live display text.
		summary := WorkOperation{Version: r.Version, ID: r.ID, ParentID: r.ParentID, Kind: r.Kind, Trigger: r.Trigger, Stack: r.Stack, Revision: r.Revision, Status: r.Status, Message: "Operation " + r.Status, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt}
		if r.Status == "running" && len(r.Steps) > 0 {
			switch r.Steps[len(r.Steps)-1].Phase {
			case "waiting":
				summary.Message = "Waiting for execution lock"
			case "executing":
				summary.Message = "Executing stack deployment"
			case "pulling":
				summary.Message = "Pulling images declared by this stack"
			case "deploying", "applying":
				summary.Message = "Applying Compose configuration"
			case "pruning":
				summary.Message = "Pruning unused images on the Docker endpoint"
			case "stack":
				summary.Message = "Stack update result recorded"
			}
		}
		chain = append(chain, summary)
		if r.ParentID == "" {
			root = r.ID
		}
		id = r.ParentID
	}
	for i := range chain {
		chain[i].RootOperationID = root
		b.work.operations[chain[i].ID] = chain[i]
	}
	return chain
}

func (b *Broadcaster) publishOperationLocked(r operations.Record) {
	b.publishLocked(EventType("operation.upsert"), r)
	chain := b.operationChainLocked(r.ID)
	b.nextWorkSequenceLocked()
	b.publishLocked(EventWorkUpsert, WorkUpsert{SchemaVersion: 2, Epoch: b.work.epoch, Sequence: b.work.sequence, Operations: chain})
	// Metadata transitions also flush the latest pending bytes immediately.
	for _, run := range b.work.runs {
		if run.OperationID == r.ID && run.timer != nil {
			b.emitRunLocked(run)
		}
	}
	b.pruneWorkOperationsLocked()
}

func (b *Broadcaster) pruneWorkOperationsLocked() {
	keep := make(map[string]bool)
	var mark func(string)
	mark = func(id string) {
		if id == "" || keep[id] {
			return
		}
		keep[id] = true
		if op, ok := b.work.operations[id]; ok {
			mark(op.ParentID)
		}
	}
	for id, op := range b.work.operations {
		if op.Status == "running" {
			mark(id)
		}
	}
	for _, run := range b.work.runs {
		mark(run.OperationID)
	}
	for id := range b.work.operations {
		if !keep[id] {
			delete(b.work.operations, id)
		}
	}
}

func (b *Broadcaster) workSnapshotLocked() WorkSnapshot {
	b.boundWorkDetailsLocked()
	s := WorkSnapshot{SchemaVersion: 2, Epoch: b.work.epoch, Sequence: b.work.sequence, Operations: []WorkOperation{}, Runs: []WorkRun{}}
	for _, op := range b.work.operations {
		s.Operations = append(s.Operations, op)
	}
	sort.Slice(s.Operations, func(i, j int) bool { return s.Operations[i].ID < s.Operations[j].ID })
	for _, run := range b.work.runs {
		s.Runs = append(s.Runs, run.WorkRun)
		s.DetailsTruncated = s.DetailsTruncated || run.DetailsTruncated
	}
	sort.Slice(s.Runs, func(i, j int) bool { return s.Runs[i].RunID < s.Runs[j].RunID })
	return s
}

// WorkOptions identifies an actual execution boundary, never a browser request.
type WorkOptions struct{ Source, Stack, Revision, CycleID, Phase string }

func (b *Broadcaster) BeginWork(ctx context.Context, opts WorkOptions) string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	chain := b.operationChainLocked(operations.ID(ctx))
	if len(chain) == 0 || chain[0].RootOperationID == "" {
		return ""
	}
	if opts.Source != "scheduler" && opts.Source != "reconcile" && opts.Source != "refresh" {
		return ""
	}
	if !runningPhase(opts.Phase) {
		return ""
	}
	now := time.Now().UTC()
	run := &liveRun{WorkRun: WorkRun{RunID: uuid.NewString(), OperationID: chain[0].ID, RootOperationID: chain[0].RootOperationID, ParentOperationID: chain[0].ParentID, Source: opts.Source, Stack: opts.Stack, Revision: opts.Revision, CycleID: opts.CycleID, Status: "running", Phase: opts.Phase, StartedAt: now, UpdatedAt: now, Images: []WorkImage{}}}
	run.layersSeen = make(map[[sha256.Size]byte]struct{})
	b.work.runs[run.RunID] = run
	b.emitRunLocked(run)
	return run.RunID
}

func runningPhase(phase string) bool {
	switch phase {
	case "waiting", "checking_repository", "pulling", "applying", "pruning":
		return true
	}
	return false
}

func (b *Broadcaster) WorkPhase(id, phase, message string) {
	if b == nil || !runningPhase(phase) {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if run := b.work.runs[id]; run != nil {
		run.Phase, run.Message, run.UpdatedAt = phase, safeWorkText(message), time.Now().UTC()
		b.emitRunLocked(run)
	}
}

// EndWork publishes the immutable terminal replacement, then releases all detail
// and the timer. Late callbacks cannot recreate a run or alter its result.
func (b *Broadcaster) EndWork(id, status, message, code string) {
	if b == nil {
		return
	}
	if status != "completed" && status != "failed" && status != "cancelled" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	run := b.work.runs[id]
	if run == nil {
		return
	}
	now := time.Now().UTC()
	run.Status, run.Phase, run.Message, run.ErrorCode = status, "finished", safeWorkText(message), code
	run.UpdatedAt, run.FinishedAt = now, &now
	for i := range run.Images {
		if !run.Images[i].Done {
			run.Images[i].Status, run.Images[i].Done = status, true
		}
	}
	b.emitRunLocked(run)
	b.work.censusEntries -= len(run.layersSeen)
	b.work.detailBytes -= detailBytes(run)
	delete(b.work.runs, id)
	b.pruneWorkOperationsLocked()
}

// WorkPullProgress is scoped by an execution run and the puller's one-based
// invocation position. References/layer IDs are never cross-run cache keys.
type WorkPullProgress struct {
	Image, LayerID, Status   string
	Current, Total           int
	BytesCurrent, BytesTotal int64
	Done                     bool
}

func (b *Broadcaster) WorkPullProgress(id string, p WorkPullProgress) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	run := b.work.runs[id]
	if run == nil || run.Phase != "pulling" || p.Image == "" {
		return
	}
	if p.Current < 0 || p.Total < 0 || int64(p.Current) > 9007199254740991 || int64(p.Total) > 9007199254740991 || (p.Total > 0 && p.Current > p.Total) {
		return
	}
	index := -1
	for i := range run.Images {
		if (p.Current > 0 && run.Images[i].Position == p.Current) || ((p.Current == 0 || run.Images[i].Position == 0) && run.Images[i].Reference == p.Image) {
			index = i
			break
		}
	}
	immediate := false
	if index < 0 {
		// Once collection is truncated, do not retain an unbounded shadow census.
		// Known cardinality remains the observed pre-truncation cardinality.
		if len(run.Images) >= WorkImageLimit || run.imagesClosed {
			// Pull positions are invocation ordinals, so sequential pulls can
			// preserve exact image cardinality without retaining omitted refs.
			if p.Current > run.ImageCount {
				run.ImageCount = p.Current
			} else if !run.imagesClosed {
				run.ImageCount++
			}
			run.imagesClosed, run.DetailsTruncated = true, true
			b.emitRunLocked(run)
			return
		}
		image := WorkImage{PullAttemptID: uuid.NewString(), Reference: p.Image, Status: "running", Position: p.Current, Layers: []WorkLayer{}}
		if p.Total > 0 {
			total := p.Total
			image.Total = &total
		}
		run.Images = append(run.Images, image)
		run.ImageCount++
		index, immediate = len(run.Images)-1, true
	}
	image := &run.Images[index]
	if image.Done || image.Reference != p.Image {
		return
	}
	if image.Position == 0 && p.Current > 0 {
		image.Position, immediate = p.Current, true
	}
	if p.Total > 0 && (image.Total == nil || *image.Total != p.Total) {
		total := p.Total
		image.Total, immediate = &total, true
	}
	now := time.Now().UTC()
	byteDelta := 0
	if p.Done {
		image.Done, image.Status, immediate = true, "completed", true
	}
	if p.LayerID != "" {
		layerIndex := -1
		for i := range image.Layers {
			if image.Layers[i].ID == p.LayerID {
				layerIndex = i
				break
			}
		}
		known := layerIndex >= 0
		if !known {
			key := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s", image.PullAttemptID, p.LayerID)))
			_, known = run.layersSeen[key]
			if !known && b.work.censusEntries < workCensusLimit {
				run.layersSeen[key] = struct{}{}
				b.work.censusEntries++
				image.LayerCount++
				known = true
			}
		}
		if layerIndex < 0 && !image.DetailsTruncated {
			if len(image.Layers) < WorkLayerLimit && known {
				image.Layers = append(image.Layers, WorkLayer{ID: p.LayerID})
				layerIndex = len(image.Layers) - 1
				immediate = true
			} else {
				image.DetailsTruncated, run.DetailsTruncated, immediate = true, true, true
			}
		}
		if layerIndex >= 0 {
			layer := &image.Layers[layerIndex]
			before := mutableLayerBytes(*layer)
			phase, done := layerPhase(p.Status)
			if layer.Phase != phase || layer.Status != safeLayerStatus(p.Status) || layer.Done != done {
				immediate = true
			}
			layer.Phase, layer.Status, layer.Done, layer.UpdatedAt = phase, safeLayerStatus(p.Status), done, now
			// Only byte-bearing phases have byte observations. A phase switch
			// clears old values instead of mixing download/extraction totals.
			layer.BytesCurrent, layer.BytesTotal = nil, nil
			if phase == "downloading" || phase == "extracting" {
				if p.BytesCurrent >= 0 && p.BytesCurrent <= 9007199254740991 {
					v := p.BytesCurrent
					layer.BytesCurrent = &v
				}
				if p.BytesTotal > 0 && p.BytesTotal <= 9007199254740991 {
					v := p.BytesTotal
					layer.BytesTotal = &v
				}
			}
			byteDelta = mutableLayerBytes(*layer) - before
		}
	}
	sort.Slice(image.Layers, func(i, j int) bool { return image.Layers[i].ID < image.Layers[j].ID })
	run.UpdatedAt = now
	// Insertions/phase changes validate at their immediate delivery barrier.
	// Byte-only changes account just bounded mutable fields, never the (possibly
	// large) layer ID or the rest of the live set. Retention stays within the
	// exact JSON budget even between deliveries; only crossing the cap evicts.
	if immediate {
		b.emitRunLocked(run)
	} else {
		b.work.detailBytes += byteDelta
		if b.work.detailBytes > WorkDetailBudget {
			b.boundWorkDetailsLocked()
		}
		b.queueRunLocked(run)
	}
}

func layerPhase(status string) (string, bool) {
	switch strings.ToLower(status) {
	case "waiting", "pulling fs layer":
		return "waiting", false
	case "downloading":
		return "downloading", false
	case "extracting":
		return "extracting", false
	case "pull complete", "already exists", "download complete":
		return "finished", true
	default:
		return "unknown", false
	}
}

// WorkPullFinished records an invocation return even when a puller produced no
// callbacks (including failure before the first Docker stream message). It does
// not end a deployment run: Compose apply still belongs to that same attempt.
func (b *Broadcaster) WorkPullFinished(id string, references []string, err error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	run := b.work.runs[id]
	if run == nil {
		return
	}
	for position, reference := range references {
		index := -1
		for i := range run.Images {
			if run.Images[i].Position == position+1 || (run.Images[i].Position == 0 && run.Images[i].Reference == reference) {
				index = i
				break
			}
		}
		if index >= 0 && run.Images[index].Done {
			continue
		}
		status := "completed"
		if err != nil {
			status = "failed"
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				status = "cancelled"
			}
		}
		if index >= 0 {
			run.Images[index].Status, run.Images[index].Done = status, true
		} else if !run.imagesClosed && len(run.Images) < WorkImageLimit {
			total := len(references)
			run.Images = append(run.Images, WorkImage{PullAttemptID: uuid.NewString(), Reference: reference, Status: status, Done: true, Position: position + 1, Total: &total, Layers: []WorkLayer{}})
			run.ImageCount++
		} else {
			if position+1 > run.ImageCount {
				run.ImageCount = position + 1
			}
			run.imagesClosed, run.DetailsTruncated = true, true
		}
		// A sequential pull returns at the first failure; later configured
		// references were never attempted and must not become failed images.
		if err != nil {
			break
		}
	}
	run.UpdatedAt = time.Now().UTC()
	b.emitRunLocked(run)
}

func safeLayerStatus(status string) string {
	// Docker's unknown status may contain registry errors/credentials. Preserve
	// recognized display statuses only; never echo arbitrary daemon text.
	if phase, _ := layerPhase(status); phase != "unknown" {
		return safeWorkText(status)
	}
	return "Pull progress"
}

func safeWorkText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if len(s) > 1024 {
		s = s[:1024]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

func (b *Broadcaster) queueRunLocked(run *liveRun) {
	if run.timer != nil {
		return
	}
	delay := time.Until(run.lastSent.Add(workByteInterval))
	if delay <= 0 {
		b.emitRunLocked(run)
		return
	}
	id := run.RunID
	run.timer = time.AfterFunc(delay, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if current := b.work.runs[id]; current == run && current.timer != nil {
			b.emitRunLocked(current)
		}
	})
}

func (b *Broadcaster) emitRunLocked(run *liveRun) {
	if run.timer != nil {
		run.timer.Stop()
		run.timer = nil
	}
	b.boundWorkDetailsLocked()
	b.nextWorkSequenceLocked()
	run.lastSent = time.Now()
	b.publishLocked(EventWorkUpsert, WorkUpsert{SchemaVersion: 2, Epoch: b.work.epoch, Sequence: b.work.sequence, Operations: b.operationChainLocked(run.OperationID), Run: &run.WorkRun})
}

func (b *Broadcaster) nextWorkSequenceLocked() {
	if b.work.sequence >= 9007199254740991 {
		// Clients adopt epochs only from a fresh subscription barrier.
		for sub := range b.subscribers {
			sub.Close()
			delete(b.subscribers, sub)
		}
		b.work.epoch, b.work.sequence = uuid.NewString(), 0
	}
	b.work.sequence++
}

// SetWorkStackResults transfers a bounded aggregate to an actual scheduler run.
// Callers can accumulate using AddWorkStackOutcome without retaining references
// for all discovered stacks. Counters are never inferred from discovered totals.
func AddWorkStackOutcome(results *WorkStackResults, outcome WorkStackOutcome) {
	if results.Outcomes == nil {
		results.Outcomes = []WorkStackOutcome{}
	}
	results.OutcomeCount++
	if outcome.Status == "skipped" {
		results.Skipped++
	} else {
		results.Processed++
	}
	if outcome.Status == "failed" {
		results.Failed++
	}
	outcome.Message = safeWorkText(outcome.Message)
	if len(results.Outcomes) < WorkOutcomeLimit {
		results.Outcomes = append(results.Outcomes, outcome)
	} else {
		results.DetailsTruncated = true
	}
}

func (b *Broadcaster) SetWorkStackResults(id string, results WorkStackResults) {
	if b == nil {
		return
	}
	if results.Processed < 0 || results.Skipped < 0 || results.Failed < 0 || results.Failed > results.Processed || results.OutcomeCount < len(results.Outcomes) || results.OutcomeCount != results.Processed+results.Skipped {
		return
	}
	for _, outcome := range results.Outcomes {
		if outcome.Stack == "" || (outcome.Status != "completed" && outcome.Status != "skipped" && outcome.Status != "failed") {
			return
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if run := b.work.runs[id]; run != nil {
		results.Outcomes = append([]WorkStackOutcome{}, results.Outcomes...)
		if len(results.Outcomes) > WorkOutcomeLimit {
			results.Outcomes = results.Outcomes[:WorkOutcomeLimit]
			results.DetailsTruncated = true
		}
		if results.OutcomeCount > len(results.Outcomes) {
			results.DetailsTruncated = true
		}
		for i := range results.Outcomes {
			results.Outcomes[i].Message = safeWorkText(results.Outcomes[i].Message)
		}
		run.StackResults = &results
		run.DetailsTruncated = run.DetailsTruncated || results.DetailsTruncated
		b.emitRunLocked(run)
	}
}

func detailBytes(run *liveRun) int {
	images, _ := json.Marshal(run.Images)
	n := len(images)
	if run.StackResults != nil {
		outcomes, _ := json.Marshal(run.StackResults.Outcomes)
		n += len(outcomes)
	}
	return n
}

func mutableLayerBytes(layer WorkLayer) int {
	// JSON field punctuation is constant, so excluding only the immutable ID
	// yields the exact size delta, including optional numbers and time precision.
	layer.ID = ""
	encoded, _ := json.Marshal(layer)
	return len(encoded)
}

// Evict actual cached detail, not just its serialized snapshot. Stable run/ID
// ordering makes retention deterministic; counts and mandatory summaries survive.
func (b *Broadcaster) boundWorkDetailsLocked() {
	ids := make([]string, 0, len(b.work.runs))
	total := 0
	for id, run := range b.work.runs {
		ids = append(ids, id)
		total += detailBytes(run)
	}
	defer func() { b.work.detailBytes = total }()
	if total <= WorkDetailBudget {
		return
	}
	sort.Strings(ids)
	for n := len(ids) - 1; n >= 0 && total > WorkDetailBudget; n-- {
		run := b.work.runs[ids[n]]
		before := detailBytes(run)
		for i := len(run.Images) - 1; i >= 0 && total > WorkDetailBudget; i-- {
			image := &run.Images[i]
			if len(image.Layers) > 0 {
				image.Layers = []WorkLayer{}
				image.DetailsTruncated, run.DetailsTruncated = true, true
				after := detailBytes(run)
				total -= before - after
				before = after
			}
		}
	}
	for n := len(ids) - 1; n >= 0 && total > WorkDetailBudget; n-- {
		run := b.work.runs[ids[n]]
		before := detailBytes(run)
		if len(run.Images) > 0 {
			run.Images = []WorkImage{}
			run.DetailsTruncated = true
			run.imagesClosed = true
		}
		if run.StackResults != nil && len(run.StackResults.Outcomes) > 0 {
			run.StackResults.Outcomes = []WorkStackOutcome{}
			run.StackResults.DetailsTruncated, run.DetailsTruncated = true, true
		}
		total -= before - detailBytes(run)
	}
}

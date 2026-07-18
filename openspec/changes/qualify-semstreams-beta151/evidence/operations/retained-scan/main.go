package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	bucketName = graph.BucketEntityStates
	streamName = "KV_" + bucketName
	keyPrefix  = "$KV." + bucketName + "."
)

type record struct {
	Sequence    uint64 `json:"sequence"`
	Subject     string `json:"subject"`
	Key         string `json:"key"`
	Operation   string `json:"operation"`
	Timestamp   string `json:"timestamp"`
	Bytes       int    `json:"bytes"`
	SHA256      string `json:"sha256"`
	EntityID    string `json:"entityId,omitempty"`
	TripleCount int    `json:"tripleCount,omitempty"`
	Valid       bool   `json:"valid"`
	Violation   string `json:"violation,omitempty"`
}

type foreignEdge struct {
	Sequence  uint64 `json:"sequence"`
	Current   bool   `json:"current"`
	SourceKey string `json:"sourceKey"`
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
}

type report struct {
	FormatVersion          string        `json:"formatVersion"`
	Validator              string        `json:"validator"`
	CapturedAt             string        `json:"capturedAt"`
	NATSServerID           string        `json:"natsServerId"`
	Bucket                 string        `json:"bucket"`
	Stream                 string        `json:"stream"`
	StreamSubjects         []string      `json:"streamSubjects"`
	History                int64         `json:"history"`
	BackingStore           string        `json:"backingStore"`
	StreamMessages         uint64        `json:"streamMessages"`
	StreamBytes            uint64        `json:"streamBytes"`
	FirstSequence          uint64        `json:"firstSequence"`
	LastSequence           uint64        `json:"lastSequence"`
	StreamSubjectsCount    uint64        `json:"streamSubjectsCount"`
	DeletedSequenceGaps    int           `json:"deletedSequenceGaps"`
	CurrentKeyCount        int           `json:"currentKeyCount"`
	RetainedRecordsFound   int           `json:"retainedRecordsFound"`
	RetainedRecordsScanned int           `json:"retainedRecordsScanned"`
	TombstoneRecords       int           `json:"tombstoneRecords"`
	ViolationCount         int           `json:"violationCount"`
	MissingSequenceCount   int           `json:"missingSequenceCount"`
	CoverageComplete       bool          `json:"coverageComplete"`
	ZeroPoison             bool          `json:"zeroPoison"`
	CurrentKeys            []string      `json:"currentKeys"`
	Records                []record      `json:"records"`
	HostedByForeignEdges   []foreignEdge `json:"hostedByForeignEdges"`
}

func main() {
	natsURL := flag.String("nats-url", "nats://127.0.0.1:4222", "NATS URL")
	output := flag.String("output", "", "output JSON path")
	flag.Parse()
	if *output == "" {
		fatal(errors.New("-output is required"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	nc, err := nats.Connect(*natsURL)
	if err != nil {
		fatal(fmt.Errorf("connect NATS: %w", err))
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		fatal(fmt.Errorf("create JetStream: %w", err))
	}
	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		fatal(fmt.Errorf("open stream %s: %w", streamName, err))
	}
	info, err := stream.Info(ctx, jetstream.WithDeletedDetails(true))
	if err != nil {
		fatal(fmt.Errorf("stream info: %w", err))
	}
	kv, err := js.KeyValue(ctx, bucketName)
	if err != nil {
		fatal(fmt.Errorf("open bucket %s: %w", bucketName, err))
	}
	status, err := kv.Status(ctx)
	if err != nil {
		fatal(fmt.Errorf("bucket status: %w", err))
	}
	keys, err := kv.Keys(ctx)
	if err != nil {
		fatal(fmt.Errorf("list current keys: %w", err))
	}
	sort.Strings(keys)
	currentRevisions := make(map[string]uint64, len(keys))
	for _, key := range keys {
		entry, getErr := kv.Get(ctx, key)
		if getErr != nil {
			fatal(fmt.Errorf("read current key %s: %w", key, getErr))
		}
		currentRevisions[key] = entry.Revision()
	}

	r := report{
		FormatVersion:       "1.0.0",
		Validator:           "graph.UnmarshalEntityState (beta.151 authoritative decoder)",
		CapturedAt:          time.Now().UTC().Format(time.RFC3339Nano),
		NATSServerID:        nc.ConnectedServerId(),
		Bucket:              status.Bucket(),
		Stream:              info.Config.Name,
		StreamSubjects:      append([]string(nil), info.Config.Subjects...),
		History:             status.History(),
		BackingStore:        status.BackingStore(),
		StreamMessages:      info.State.Msgs,
		StreamBytes:         info.State.Bytes,
		FirstSequence:       info.State.FirstSeq,
		LastSequence:        info.State.LastSeq,
		StreamSubjectsCount: info.State.NumSubjects,
		DeletedSequenceGaps: info.State.NumDeleted,
		CurrentKeyCount:     len(keys),
		CurrentKeys:         keys,
	}

	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		msg, getErr := stream.GetMsg(ctx, seq)
		if errors.Is(getErr, jetstream.ErrMsgNotFound) {
			r.MissingSequenceCount++
			continue
		}
		if getErr != nil {
			fatal(fmt.Errorf("read retained sequence %d: %w", seq, getErr))
		}
		r.RetainedRecordsFound++
		op := msg.Header.Get("KV-Operation")
		if op == "" {
			op = "PUT"
		}
		h := sha256.Sum256(msg.Data)
		rec := record{
			Sequence:  msg.Sequence,
			Subject:   msg.Subject,
			Key:       strings.TrimPrefix(msg.Subject, keyPrefix),
			Operation: op,
			Timestamp: msg.Time.UTC().Format(time.RFC3339Nano),
			Bytes:     len(msg.Data),
			SHA256:    hex.EncodeToString(h[:]),
		}
		if op != "PUT" {
			r.TombstoneRecords++
			rec.Valid = true
			r.Records = append(r.Records, rec)
			continue
		}
		r.RetainedRecordsScanned++
		var state graph.EntityState
		if err := graph.UnmarshalEntityState(msg.Data, &state); err != nil {
			rec.Violation = err.Error()
			r.ViolationCount++
		} else if state.ID != rec.Key {
			rec.Violation = fmt.Sprintf("KV key %q does not equal entity ID %q", rec.Key, state.ID)
			r.ViolationCount++
		} else {
			rec.Valid = true
			rec.EntityID = state.ID
			rec.TripleCount = len(state.Triples)
			for _, triple := range state.Triples {
				if triple.Predicate == "sensorml.component.is-hosted-by" {
					r.HostedByForeignEdges = append(r.HostedByForeignEdges, foreignEdge{
						Sequence:  msg.Sequence,
						Current:   currentRevisions[rec.Key] == msg.Sequence,
						SourceKey: rec.Key,
						Subject:   triple.Subject,
						Predicate: triple.Predicate,
						Object:    fmt.Sprint(triple.Object),
					})
				}
			}
		}
		r.Records = append(r.Records, rec)
	}

	r.CoverageComplete = uint64(r.RetainedRecordsFound) == r.StreamMessages &&
		r.RetainedRecordsScanned+r.TombstoneRecords == r.RetainedRecordsFound
	r.ZeroPoison = r.ViolationCount == 0
	if !r.CoverageComplete || !r.ZeroPoison || r.Bucket != bucketName || r.Stream != streamName {
		writeReport(*output, r)
		fatal(fmt.Errorf("retained scan failed: coverage=%t poison=%d bucket=%s stream=%s",
			r.CoverageComplete, r.ViolationCount, r.Bucket, r.Stream))
	}
	writeReport(*output, r)
	fmt.Printf("retained scan passed: stream=%s bucket=%s records=%d scanned=%d tombstones=%d current_keys=%d last_revision=%d violations=%d\n",
		r.Stream, r.Bucket, r.RetainedRecordsFound, r.RetainedRecordsScanned,
		r.TombstoneRecords, r.CurrentKeyCount, r.LastSequence, r.ViolationCount)
}

func writeReport(path string, r report) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fatal(fmt.Errorf("marshal report: %w", err))
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatal(fmt.Errorf("write report: %w", err))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

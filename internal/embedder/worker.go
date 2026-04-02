package embedder

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/thuongh2/git-mimir/internal/store"
)

const (
	batchSize    = 64
	batchTimeout = 100 * time.Millisecond
)

// EmbedJob is a single embedding request.
type EmbedJob struct {
	UID  string
	Text string
}

// Worker processes embedding jobs asynchronously in the background.
type Worker struct {
	queue chan EmbedJob
	s     *store.Store
	emb   Embedder
	wg    sync.WaitGroup
}

// NewWorker creates a new embedding worker.
func NewWorker(s *store.Store, emb Embedder) *Worker {
	return &Worker{
		// Large buffer to handle repos with 100k+ symbols without dropping jobs
		queue: make(chan EmbedJob, 100000),
		s:     s,
		emb:   emb,
	}
}

// Enqueue adds a job to the embedding queue. Non-blocking; drops if full.
func (w *Worker) Enqueue(job EmbedJob) {
	select {
	case w.queue <- job:
	default:
		// Queue full — skip (embeddings are best-effort)
		log.Printf("embedder: job dropped for %s (queue full)", job.UID)
	}
}

// Start runs the worker in a background goroutine until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			batch, ok := w.collectBatch(ctx)
			if !ok {
				// Process any remaining jobs, then exit
				if len(batch) > 0 {
					w.processBatch(batch)
				}
				return
			}
			if len(batch) > 0 {
				w.processBatch(batch)
			}
		}
	}()
}

// Close signals the worker to finish processing and waits for completion.
func (w *Worker) Close() {
	close(w.queue)
	w.wg.Wait()
}

// collectBatch gathers up to batchSize jobs or waits up to batchTimeout.
// Returns (batch, true) normally; (batch, false) when context is done.
func (w *Worker) collectBatch(ctx context.Context) ([]EmbedJob, bool) {
	timer := time.NewTimer(batchTimeout)
	defer timer.Stop()

	var batch []EmbedJob
	for {
		select {
		case <-ctx.Done():
			return batch, false
		case job, ok := <-w.queue:
			if !ok {
				return batch, false
			}
			batch = append(batch, job)
			if len(batch) >= batchSize {
				return batch, true
			}
		case <-timer.C:
			return batch, true
		}
	}
}

func (w *Worker) processBatch(jobs []EmbedJob) {
	texts := make([]string, len(jobs))
	for i, j := range jobs {
		texts[i] = j.Text
	}

	// Check cache first
	uncachedIdx := make([]int, 0, len(jobs))
	result := make(map[string][]float32, len(jobs))

	for i, j := range jobs {
		hash := TextHash(j.Text)
		cached, err := w.s.GetEmbedCache(hash)
		if err == nil && cached != nil {
			result[j.UID] = cached
		} else {
			uncachedIdx = append(uncachedIdx, i)
		}
	}

	if len(uncachedIdx) > 0 {
		uncachedTexts := make([]string, len(uncachedIdx))
		for i, idx := range uncachedIdx {
			uncachedTexts[i] = jobs[idx].Text
		}

		embeddings, err := w.emb.Embed(uncachedTexts)
		if err != nil {
			log.Printf("embedder: batch embed error: %v", err)
			return
		}

		for i, idx := range uncachedIdx {
			if i >= len(embeddings) {
				break
			}
			job := jobs[idx]
			emb := embeddings[i]
			result[job.UID] = emb
			// Cache it
			hash := TextHash(job.Text)
			if err := w.s.UpsertEmbedCache(hash, emb); err != nil {
				log.Printf("embedder: cache error for %s: %v", job.UID, err)
			}
		}
	}

	if err := w.s.BatchUpdateEmbeddings(result); err != nil {
		log.Printf("embedder: store embeddings error: %v", err)
	}
}

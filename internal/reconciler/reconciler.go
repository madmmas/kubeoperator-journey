// Package reconciler simulates the core Kubernetes controller pattern:
// Watch → Enqueue → Reconcile.
package reconciler

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/madmmas/kubeoperator-journey/internal/watcher"
)

// DesiredState is what the user declared — the spec in their CRD.
type DesiredState struct {
	Name     string
	Replicas int
	Version  string
}

// ActualState is what's running in the cluster right now.
type ActualState struct {
	Name            string
	RunningReplicas int
	CurrentVersion  string
	Healthy         bool
}

// ReconcileResult tells the control loop what to do after a reconciliation.
type ReconcileResult struct {
	Requeue      bool
	RequeueAfter time.Duration
}

// Controller implements the Watch→Enqueue→Reconcile pattern.
type Controller struct {
	store          *watcher.Store
	actual         map[string]*ActualState
	mu             sync.Mutex
	workQueue      chan string
	reconcileCount int
	stopCh         chan struct{}
}

func NewController(store *watcher.Store) *Controller {
	return &Controller{
		store:     store,
		actual:    make(map[string]*ActualState),
		workQueue: make(chan string, 64),
		stopCh:    make(chan struct{}),
	}
}

// Start launches the watch and reconcile goroutines.
func (c *Controller) Start() {
	fmt.Println("[controller] Starting watch loop and reconcile loop")
	go c.watchLoop()
	go c.reconcileLoop()
}

// watchLoop subscribes to store events and enqueues affected keys.
func (c *Controller) watchLoop() {
	events := c.store.Watch()
	for {
		select {
		case evt := <-events:
			fmt.Printf("[watch]  %s event for %q → enqueuing\n", evt.Type, evt.Key)
			c.enqueue(evt.Key)
		case <-c.stopCh:
			return
		}
	}
}

func (c *Controller) enqueue(key string) {
	select {
	case c.workQueue <- key:
	default:
		fmt.Printf("[queue]  WARNING: work queue full, dropping key=%q\n", key)
	}
}

// reconcileLoop drains the work queue, calling Reconcile for each key.
func (c *Controller) reconcileLoop() {
	for {
		select {
		case key := <-c.workQueue:
			result, err := c.Reconcile(key)
			if err != nil {
				fmt.Printf("[reconcile] ERROR for %q: %v — requeuing with backoff\n", key, err)
				go func(k string) {
					time.Sleep(2 * time.Second)
					c.enqueue(k)
				}(key)
				continue
			}
			if result.Requeue {
				go func(k string, after time.Duration) {
					time.Sleep(after)
					c.enqueue(k)
				}(key, result.RequeueAfter)
			}
		case <-c.stopCh:
			return
		}
	}
}

// Reconcile is the function you implement in Kubebuilder.
// fetch desired → observe actual → compare and act → return result
func (c *Controller) Reconcile(key string) (ReconcileResult, error) {
	c.mu.Lock()
	c.reconcileCount++
	count := c.reconcileCount
	c.mu.Unlock()

	fmt.Printf("\n[reconcile #%d] key=%q\n", count, key)

	// Step 1: Fetch desired state
	raw, exists := c.store.Get(key)
	if !exists {
		fmt.Printf("[reconcile #%d] resource %q not found — may have been deleted\n", count, key)
		c.mu.Lock()
		delete(c.actual, key)
		c.mu.Unlock()
		return ReconcileResult{}, nil
	}

	desired, ok := raw.(*DesiredState)
	if !ok {
		return ReconcileResult{}, fmt.Errorf("unexpected type in store for key %q", key)
	}
	fmt.Printf("[reconcile #%d] desired: replicas=%d version=%s\n", count, desired.Replicas, desired.Version)

	// Step 2: Observe actual state
	c.mu.Lock()
	actual, hasActual := c.actual[key]
	if !hasActual {
		actual = &ActualState{Name: key}
	}
	c.mu.Unlock()
	fmt.Printf("[reconcile #%d] actual:  replicas=%d version=%q healthy=%v\n",
		count, actual.RunningReplicas, actual.CurrentVersion, actual.Healthy)

	// Step 3: Compare and act
	needsRequeue := false

	if actual.CurrentVersion != desired.Version {
		fmt.Printf("[reconcile #%d] ACTION: upgrading %q → %s (was %q)\n",
			count, key, desired.Version, actual.CurrentVersion)
		if err := c.simulateUpgrade(key, desired.Version); err != nil {
			return ReconcileResult{}, fmt.Errorf("upgrade failed: %w", err)
		}
		c.mu.Lock()
		actual.CurrentVersion = desired.Version
		c.actual[key] = actual
		c.mu.Unlock()
		needsRequeue = true
	}

	if actual.RunningReplicas != desired.Replicas {
		fmt.Printf("[reconcile #%d] ACTION: scaling %q from %d → %d replicas\n",
			count, key, actual.RunningReplicas, desired.Replicas)
		c.mu.Lock()
		actual.RunningReplicas = desired.Replicas
		actual.Healthy = true
		c.actual[key] = actual
		c.mu.Unlock()
		needsRequeue = true
	}

	if !actual.Healthy && actual.RunningReplicas > 0 {
		fmt.Printf("[reconcile #%d] ACTION: recovering unhealthy cluster %q\n", count, key)
		c.mu.Lock()
		actual.Healthy = true
		c.actual[key] = actual
		c.mu.Unlock()
	}

	// Step 4: Done or requeue
	if !needsRequeue {
		fmt.Printf("[reconcile #%d] ✓ %q is in desired state — no action needed\n", count, key)
		return ReconcileResult{}, nil
	}

	fmt.Printf("[reconcile #%d] requeuing %q in 500ms to verify changes\n", count, key)
	return ReconcileResult{Requeue: true, RequeueAfter: 500 * time.Millisecond}, nil
}

func (c *Controller) simulateUpgrade(key, newVersion string) error {
	time.Sleep(300 * time.Millisecond)
	if rand.Float32() < 0.15 {
		return fmt.Errorf("node not ready during rolling restart of %q", key)
	}
	return nil
}

func (c *Controller) Stop() { close(c.stopCh) }

func (c *Controller) ReconcileCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reconcileCount
}

func (c *Controller) ActualStateFor(key string) (*ActualState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.actual[key]
	return s, ok
}

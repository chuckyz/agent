// Package flow implements the Flow component graph system. Flow configuration
// files are parsed from River, which contain a listing of components to run.
//
// # Components
//
// Each component has a set of arguments (River attributes and blocks) and
// optionally a set of exported fields. Components can reference the exports of
// other components using River expressions.
//
// See the top-level component package for more information on components, and
// subpackages for defined components.
//
// # Component Health
//
// A component will have various health states during its lifetime:
//
//  1. Unknown:   The initial health state for new components.
//  2. Healthy:   A healthy component
//  3. Unhealthy: An unhealthy component.
//  4. Exited:    A component which is no longer running.
//
// Health states are paired with a time for when the health state was generated
// and a message providing more detail for the health state.
//
// Components can report their own health states. The health state reported by
// a component is merged with the Flow-level health of that component: an error
// when evaluating the configuration for a component will always be reported as
// unhealthy until the next successful evaluation.
//
// # Component Evaluation
//
// The process of converting the River block associated with a component into
// the appropriate Go struct is called "component evaluation."
//
// Components are only evaluated after all components they reference have been
// evaluated; cyclic dependencies are invalid.
//
// If a component updates its Exports at runtime, other components which directly
// or indirectly reference the updated component will have their Arguments
// re-evaluated.
//
// The arguments and exports for a component will be left in their last valid
// state if a component shuts down or is given an invalid config. This prevents
// a domino effect of a single failed component taking down other components
// which are otherwise healthy.
package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/go-kit/log/level"
	"github.com/grafana/agent/pkg/flow/internal/controller"
	"github.com/grafana/agent/pkg/flow/internal/dag"
	"github.com/grafana/agent/pkg/flow/logging"
	"github.com/prometheus/client_golang/prometheus"
)

// Options holds static options for a flow controller.
type Options struct {
	// Logger for components to use. A no-op logger will be created if this is
	// nil.
	Logger *logging.Logger

	// Directory where components can write data. Components will create
	// subdirectories for component-specific data.
	DataPath string

	// Reg is the prometheus register to use
	Reg prometheus.Registerer

	// HTTPListenAddr is the base address that the server is listening on.
	// The controller does not itself listen here, but some components
	// need to know this to set the correct targets.
	HTTPListenAddr string
}

// Flow is the Flow system.
type Flow struct {
	log  *logging.Logger
	opts Options

	updateQueue *controller.Queue
	sched       *controller.Scheduler
	loader      *controller.Loader

	cancel       context.CancelFunc
	exited       chan struct{}
	loadFinished chan struct{}

	loadMut    sync.RWMutex
	loadedOnce bool
}

// New creates and starts a new Flow controller. Call Close to stop
// the controller.
func New(o Options) *Flow {
	c, ctx := newFlow(o)
	go c.run(ctx)
	return c
}

func newFlow(o Options) (*Flow, context.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	log := o.Logger
	if log == nil {
		var err error
		log, err = logging.New(io.Discard, logging.DefaultOptions)
		if err != nil {
			// This shouldn't happen unless there's a bug
			panic(err)
		}
	}

	var (
		queue  = controller.NewQueue()
		sched  = controller.NewScheduler()
		loader = controller.NewLoader(controller.ComponentGlobals{
			Logger:   log,
			DataPath: o.DataPath,
			OnExportsChange: func(cn *controller.ComponentNode) {
				// Changed components should be queued for reevaluation.
				queue.Enqueue(cn)
			},
			Registerer:     o.Reg,
			HTTPListenAddr: o.HTTPListenAddr,
		})
	)

	return &Flow{
		log:  log,
		opts: o,

		updateQueue: queue,
		sched:       sched,
		loader:      loader,

		cancel:       cancel,
		exited:       make(chan struct{}, 1),
		loadFinished: make(chan struct{}, 1),
	}, ctx
}

func (c *Flow) run(ctx context.Context) {
	defer close(c.exited)
	defer level.Debug(c.log).Log("msg", "flow controller exiting")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return

		case <-c.updateQueue.Chan():
			// We need to pop _everything_ from the queue and evaluate each of them.
			// If we only pop a single element, other components may sit waiting for
			// evaluation forever.
			for {
				updated := c.updateQueue.TryDequeue()
				if updated == nil {
					break
				}

				level.Debug(c.log).Log("msg", "handling component with updated state", "node_id", updated.NodeID())
				c.loader.EvaluateDependencies(nil, updated)
			}

		case <-c.loadFinished:
			level.Info(c.log).Log("msg", "scheduling loaded components")

			components := c.loader.Components()
			runnables := make([]controller.RunnableNode, 0, len(components))
			for _, uc := range components {
				runnables = append(runnables, uc)
			}
			err := c.sched.Synchronize(runnables)
			if err != nil {
				level.Error(c.log).Log("msg", "failed to load components", "err", err)
			}
		}
	}
}

// LoadFile synchronizes the state of the controller with the current config
// file. Components in the graph will be marked as unhealthy if there was an
// error encountered during Load.
//
// The controller will only start running components after Load is called once
// without any configuration errors.
func (c *Flow) LoadFile(file *File) error {
	c.loadMut.Lock()
	defer c.loadMut.Unlock()

	err := c.log.Update(file.Logging)
	if err != nil {
		return fmt.Errorf("error updating logger: %w", err)
	}

	diags := c.loader.Apply(nil, file.Components)
	if !c.loadedOnce && diags.HasErrors() {
		// The first call to Load should not run any components if there were
		// errors in the configuration file.
		return diags
	}
	c.loadedOnce = true

	select {
	case c.loadFinished <- struct{}{}:
	default:
		// A refresh is already scheduled
	}
	return diags.ErrorOrNil()
}

// ComponentInfos returns the component infos.
func (c *Flow) ComponentInfos() []*ComponentInfo {
	c.loadMut.RLock()
	defer c.loadMut.RUnlock()

	cns := c.loader.Components()
	infos := make([]*ComponentInfo, len(cns))
	edges := c.loader.OriginalGraph().Edges()
	for i, com := range cns {
		nn := newFromNode(com, edges)
		infos[i] = nn
	}
	return infos
}

// Close closes the controller and all running components.
func (c *Flow) Close() error {
	c.cancel()
	<-c.exited
	return c.sched.Close()
}

func newFromNode(cn *controller.ComponentNode, edges []dag.Edge) *ComponentInfo {
	references := make([]string, 0)
	referencedBy := make([]string, 0)
	for _, e := range edges {
		if e.From.NodeID() == cn.NodeID() {
			references = append(references, e.To.NodeID())
		} else if e.To.NodeID() == cn.NodeID() {
			referencedBy = append(referencedBy, e.From.NodeID())
		}
	}
	h := cn.CurrentHealth()
	ci := &ComponentInfo{
		Label:        cn.Label(),
		ID:           cn.NodeID(),
		Name:         cn.ComponentName(),
		Type:         "block",
		References:   references,
		ReferencedBy: referencedBy,
		Health: &ComponentHealth{
			State:       h.Health.String(),
			Message:     h.Message,
			UpdatedTime: h.UpdateTime,
		},
	}
	return ci
}

// ComponentInfo represents a component in flow.
type ComponentInfo struct {
	Name         string           `json:"name,omitempty"`
	Type         string           `json:"type,omitempty"`
	ID           string           `json:"id,omitempty"`
	Label        string           `json:"label,omitempty"`
	References   []string         `json:"referencesTo"`
	ReferencedBy []string         `json:"referencedBy"`
	Health       *ComponentHealth `json:"health"`
	Original     string           `json:"original"`
	Arguments    json.RawMessage  `json:"arguments,omitempty"`
	Exports      json.RawMessage  `json:"exports,omitempty"`
	DebugInfo    json.RawMessage  `json:"debugInfo,omitempty"`
}

// ComponentHealth represents the health of a component.
type ComponentHealth struct {
	State       string    `json:"state"`
	Message     string    `json:"message"`
	UpdatedTime time.Time `json:"updatedTime"`
}

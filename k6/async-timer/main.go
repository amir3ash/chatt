package apiasync

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modules"
	"go.k6.io/k6/metrics"
)

var timeMap sync.Map

type mapKey struct {
	uuid uuid.UUID
	vuId uint64
}

// init is called by the Go runtime at application startup.
func init() {
	modules.Register("k6/x/async-timer", New())
	timeMap = sync.Map{}
}

type DurationMs = int64

type (
	// RootModule is the global module instance that will create module
	// instances for each VU.
	RootModule struct{}

	// ModuleInstance represents an instance of the JS module.
	ModuleInstance struct {
		// vu provides methods for accessing internal k6 objects for a VU
		vu modules.VU
		// comparator is the exported type
		comparator *Compare

		metrics *asyncMetrics
	}
)

// Ensure the interfaces are implemented correctly.
var (
	_ modules.Instance = &ModuleInstance{}
	_ modules.Module   = &RootModule{}
)

// New returns a pointer to a new RootModule instance.
func New() *RootModule {
	return &RootModule{}
}

// NewModuleInstance implements the modules.Module interface returning a new instance for each VU.
func (*RootModule) NewModuleInstance(vu modules.VU) modules.Instance {

	metrics, err := registerMetrics(vu)
	if err != nil {
		common.Throw(vu.Runtime(), err)
	}

	return &ModuleInstance{
		vu:         vu,
		comparator: &Compare{vu: vu},
		metrics:    &metrics,
	}
}

// Compare is the type for our custom API.
type Compare struct {
	vu               modules.VU // provides methods for accessing internal k6 objects
	ComparisonResult string     // textual description of the most recent comparison

}

// IsGreater returns true if a is greater than b, or false otherwise, setting textual result message.
func (c *Compare) IsGreater(a, b int) bool {
	if a > b {
		c.ComparisonResult = fmt.Sprintf("%d is greater than %d", a, b)
		return true
	} else {
		c.ComparisonResult = fmt.Sprintf("%d is NOT greater than %d", a, b)
		return false
	}
}

func (c *ModuleInstance) Start() string {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		common.Throw(c.vu.Runtime(), fmt.Errorf("cant create uuid: %w", err))
	}

	state := c.vu.State()
	if state == nil {
		slog.Error("state is nil")
		common.Throw(c.vu.Runtime(), fmt.Errorf("state is nil"))
	}

	return fmt.Sprintf("%d_%s", state.VUID, newUUID.String())
}

func (c *ModuleInstance) Sent(input string) {
	senderVUID, id, err := parseInput(input)
	if err != nil {
		common.Throw(c.vu.Runtime(), err)
		return
	}

	state := c.vu.State()
	if state == nil {
		slog.Error("state is nil")
		common.Throw(c.vu.Runtime(), fmt.Errorf("state is nil"))
	}

	ctx := c.vu.Context()
	if ctx == nil {
		slog.Error("context is nil")
		common.Throw(c.vu.Runtime(), fmt.Errorf("context is nil"))
	}

	ctm := state.Tags.GetCurrentValues()
	sampleTags := ctm.Tags
	now := time.Now()

	metrics.PushIfNotDone(ctx, state.Samples, metrics.Sample{
		Time: now,
		TimeSeries: metrics.TimeSeries{
			Metric: c.metrics.Count,
			Tags:   sampleTags,
		},
		Value:    1,
		Metadata: ctm.Metadata,
	})

	metrics.PushIfNotDone(ctx, state.Samples, metrics.Sample{
		Time: now,
		TimeSeries: metrics.TimeSeries{
			Metric: c.metrics.NotRecieved,
			Tags:   sampleTags,
		},
		Value:    1,
		Metadata: ctm.Metadata,
	})

	timeMap.Store(mapKey{id, senderVUID}, DurationMs(now.UnixMilli()))
}

// loads and deletes time from map, if senderVU that called start() is current VU
func (c *ModuleInstance) End(input string) {
	now := time.Now()
	senderVUID, id, err := parseInput(input)
	if err != nil {
		common.Throw(c.vu.Runtime(), err)
		return
	}

	state := c.vu.State()
	if state == nil {
		slog.Error("state is nil")
		common.Throw(c.vu.Runtime(), fmt.Errorf("state is nil"))
	}

	if senderVUID != c.vu.State().VUID {
		return
	}

	ctx := c.vu.Context()
	if ctx == nil {
		slog.Error("context is nil")
		common.Throw(c.vu.Runtime(), fmt.Errorf("context is nil"))
	}

	ctm := state.Tags.GetCurrentValues()
	sampleTags := ctm.Tags

	v, exists := timeMap.LoadAndDelete(mapKey{id, state.VUID})
	if !exists {
		metrics.PushIfNotDone(ctx, state.Samples, metrics.Sample{
			Time: now,
			TimeSeries: metrics.TimeSeries{
				Metric: c.metrics.NotFoundNum,
				Tags:   sampleTags,
			},
			Value:    1,
			Metadata: ctm.Metadata,
		})

		return
	}

	started := v.(DurationMs)
	durationMs := now.UnixMilli() - started

	metrics.PushIfNotDone(ctx, state.Samples, metrics.Sample{
		Time: now,
		TimeSeries: metrics.TimeSeries{
			Metric: c.metrics.WaitTime,
			Tags:   sampleTags,
		},
		Value:    float64(durationMs),
		Metadata: ctm.Metadata,
	})

	metrics.PushIfNotDone(ctx, state.Samples, metrics.Sample{
		Time: time.Now(),
		TimeSeries: metrics.TimeSeries{
			Metric: c.metrics.NotRecieved,
			Tags:   sampleTags,
		},
		Value:    -1,
		Metadata: ctm.Metadata,
	})
}

// Exports implements the modules.Instance interface and returns the exported types for the JS module.
func (mi *ModuleInstance) Exports() modules.Exports {
	return modules.Exports{
		Default: mi,
	}
}

func parseInput(input string) (senderVUID uint64, id uuid.UUID, err error) {
	var strUUID string
	_, err = fmt.Sscanf(input, "%d_%s", &senderVUID, &strUUID)
	if err != nil {
		err = fmt.Errorf("cant parse input: %w", err)
		return
	}

	id, err = uuid.Parse(strUUID)
	if err != nil {
		err = fmt.Errorf("cant parse uuid: %w", err)
		return
	}
	return
}

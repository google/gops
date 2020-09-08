package goprocess

import (
	"reflect"
	"sort"
	"testing"

	"github.com/keybase/go-ps"
)

func BenchmarkFindAll(b *testing.B) {
	for ii := 0; ii < b.N; ii++ {
		_ = FindAll()
	}
}

type fixtureFunc func() (pss []ps.Process, want []P, fn isGoFunc, done func())

// TestFindAll tests findAll implementation function.
func TestFindAll(t *testing.T) {
	for _, tc := range []struct {
		name           string
		minConcurrency int
		concurrency    int
		input          []ps.Process
		goPIDs         []int
		want           []P
	}{{
		name:        "no processes",
		concurrency: 10,
		input:       nil,
		want:        nil,
	}, {
		name:        "non-Go process",
		concurrency: 10,
		input:       processesWithPID(1),
		want:        nil,
	}, {
		name:        "Go process",
		concurrency: 10,
		input:       processesWithPID(1),
		goPIDs:      []int{1},
		want:        []P{{PID: 1}},
	}, {
		name:        "filters Go processes",
		concurrency: 10,
		input:       processesWithPID(1, 2, 3, 4, 5, 6, 7),
		goPIDs:      []int{1, 3, 5, 7},
		want:        []P{{PID: 1}, {PID: 3}, {PID: 5}, {PID: 7}},
	}, {
		name:           "Go processes above max concurrency (issue #123)",
		concurrency:    2,
		minConcurrency: 2,
		input:          processesWithPID(1, 2, 3, 4, 5, 6, 7),
		goPIDs:         []int{1, 3, 5, 7},
		want:           []P{{PID: 1}, {PID: 3}, {PID: 5}, {PID: 7}},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			sync, done := semaphore(tc.minConcurrency)
			defer done()

			actual := findAll(tc.input, syncedIsGo(sync, tc.goPIDs), tc.concurrency)
			sort.Slice(actual, func(i, j int) bool { return actual[i].PID < actual[j].PID })
			if !reflect.DeepEqual(actual, tc.want) {
				t.Errorf("findAll(concurrency=%v)\ngot  %v\nwant %v",
					tc.concurrency, actual, tc.want)
			}
		})
	}
}

// semaphore blocks first minConcurrency calls to sync.
func semaphore(minConcurrency int) (sync func(), done func()) {
	waitCh := make(chan chan struct{})
	done = func() { close(waitCh) }
	go func() {
		if minConcurrency > 0 {
			// Wait for minConcurrency sync calls.
			var ch []chan struct{}
			for c := range waitCh {
				ch = append(ch, c)
				if len(ch) >= minConcurrency {
					break
				}
			}
			// Release all of them.
			for _, c := range ch {
				c <- struct{}{}
			}
		}
		// Release every other sync call immediately.
		for c := range waitCh {
			c <- struct{}{}
		}
	}()
	sync = func() {
		c := make(chan struct{})
		waitCh <- c
		<-c
	}
	return
}

func syncedIsGo(sync func(), goPIDs []int) isGoFunc {
	return func(pr ps.Process) (path, version string, agent, ok bool, err error) {
		sync()
		for _, p := range goPIDs {
			if p == pr.Pid() {
				ok = true
				return
			}
		}
		return
	}
}

func processesWithPID(pids ...int) (p []ps.Process) {
	for _, pid := range pids {
		p = append(p, process{pid: pid})
	}
	return
}

type process struct {
	ps.Process
	pid int
}

func (p process) Pid() int           { return p.pid }
func (p process) PPid() int          { return 0 }
func (p process) Executable() string { return "" }

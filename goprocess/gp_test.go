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

// TestFindAll tests findAll implementation function.
func TestFindAll(t *testing.T) {
	for _, tc := range []struct {
		name             string
		concurrencyLimit int
		input            []ps.Process
		goPIDs           []int
		want             []P
	}{{
		name:             "no processes",
		concurrencyLimit: 10,
		input:            nil,
		want:             nil,
	}, {
		name:             "non-Go process",
		concurrencyLimit: 10,
		input:            fakeProcessesWithPIDs(1),
		want:             nil,
	}, {
		name:             "Go process",
		concurrencyLimit: 10,
		input:            fakeProcessesWithPIDs(1),
		goPIDs:           []int{1},
		want:             []P{{PID: 1}},
	}, {
		name:             "filters Go processes",
		concurrencyLimit: 10,
		input:            fakeProcessesWithPIDs(1, 2, 3, 4, 5, 6, 7),
		goPIDs:           []int{1, 3, 5, 7},
		want:             []P{{PID: 1}, {PID: 3}, {PID: 5}, {PID: 7}},
	}, {
		name:             "Go processes above max concurrency (issue #123)",
		concurrencyLimit: 2,
		input:            fakeProcessesWithPIDs(1, 2, 3, 4, 5, 6, 7),
		goPIDs:           []int{1, 3, 5, 7},
		want:             []P{{PID: 1}, {PID: 3}, {PID: 5}, {PID: 7}},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			actual := findAll(tc.input, fakeIsGo(tc.goPIDs), tc.concurrencyLimit)
			sort.Slice(actual, func(i, j int) bool { return actual[i].PID < actual[j].PID })
			if !reflect.DeepEqual(actual, tc.want) {
				t.Errorf("findAll(concurrency=%v)\ngot  %v\nwant %v",
					tc.concurrencyLimit, actual, tc.want)
			}
		})
	}
}

func fakeIsGo(goPIDs []int) isGoFunc {
	return func(pr ps.Process) (path, version string, agent, ok bool, err error) {
		for _, p := range goPIDs {
			if p == pr.Pid() {
				ok = true
				return
			}
		}
		return
	}
}

func fakeProcessesWithPIDs(pids ...int) []ps.Process {
	p := make([]ps.Process, 0, len(pids))
	for _, pid := range pids {
		p = append(p, fakeProcess{pid: pid})
	}
	return p
}

type fakeProcess struct {
	ps.Process
	pid int
}

func (p fakeProcess) Pid() int           { return p.pid }
func (p fakeProcess) PPid() int          { return 0 }
func (p fakeProcess) Executable() string { return "" }

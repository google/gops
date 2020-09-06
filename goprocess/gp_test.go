package goprocess

import (
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/keybase/go-ps"
)

func BenchmarkFindAll(b *testing.B) {
	for ii := 0; ii < b.N; ii++ {
		_ = FindAll()
	}
}

type fixtureFunc func() (pss []ps.Process, want []P, fn findFunc, done func())

// TestFindAll tests findAll implementation function.
func TestFindAll(t *testing.T) {
	for _, test := range []struct {
		name        string
		fixture     fixtureFunc
		concurrency int
	}{
		{
			name:    "no processes",
			fixture: fakeProcesses(0),
		},
		{
			name:        "handles many go processes (issue#123)",
			fixture:     fakeProcesses(100),
			concurrency: 10,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pss, want, findFunc, finalizeFixture := test.fixture()
			defer finalizeFixture()
			actual := findAll(pss, findFunc, test.concurrency)
			sort.Slice(actual, func(i, j int) bool { return actual[i].PID < actual[j].PID })
			if !reflect.DeepEqual(actual, want) {
				t.Errorf("findAll(concurrency=%v)\ngot  %v\nwant %v",
					test.concurrency, actual, want)
			}
		})
	}
}

func fakeProcesses(count int) fixtureFunc {
	return func() (pss []ps.Process, want []P, fn findFunc, done func()) {
		var wg sync.WaitGroup
		wg.Add(count)
		alive := make(chan struct{})

		done = func() {
			close(alive)
			wg.Wait()
		}

		for pid := 1; pid <= count; pid++ {
			pid := pid
			go func() {
				defer wg.Done()
				<-alive
			}()
			pss = append(pss, process{pid: pid})
			want = append(want, P{PID: pid})
		}

		fn = func(pid int) (p P, ok bool, err error) {
			p.PID = pid
			return p, true, nil
		}

		return
	}
}

type process struct {
	ps.Process
	pid int
}

func (p process) Pid() int { return p.pid }

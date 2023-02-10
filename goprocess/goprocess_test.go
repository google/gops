package goprocess

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"testing"

	"github.com/shirou/gopsutil/v3/process"
)

func BenchmarkFindAll(b *testing.B) {
	for ii := 0; ii < b.N; ii++ {
		_ = FindAll()
	}
}

// TestFindAll tests findAll implementation function.
func TestFindAll(t *testing.T) {
	testProcess, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		t.Errorf("failed to get current process: %v", err)
	}
	testPpid, _ := testProcess.Ppid()
	testExec, _ := testProcess.Name()
	wantProcess := P{PID: int(testProcess.Pid), PPID: int(testPpid), Exec: testExec}

	for _, tc := range []struct {
		name             string
		concurrencyLimit int
		input            []*process.Process
		goPIDs           []int
		want             []P
		mock             bool
	}{{
		name:             "no processes",
		concurrencyLimit: 10,
		input:            nil,
		want:             nil,
	}, {
		name:             "non-Go process",
		concurrencyLimit: 10,
		input:            []*process.Process{testProcess},
		want:             nil,
	}, {
		name:             "Go process",
		concurrencyLimit: 10,
		input:            []*process.Process{testProcess},
		goPIDs:           []int{int(testProcess.Pid)},
		want:             []P{wantProcess},
	}, {
		name:             "filters Go processes",
		concurrencyLimit: 10,
		input:            fakeProcessesWithPIDs(1, 2, 3, 4, 5, 6, 7),
		goPIDs:           []int{1, 3, 5, 7},
		want:             []P{{PID: 1}, {PID: 3}, {PID: 5}, {PID: 7}},
		mock:             true,
	}, {
		name:             "Go processes above max concurrency (issue #123)",
		concurrencyLimit: 2,
		input:            fakeProcessesWithPIDs(1, 2, 3, 4, 5, 6, 7),
		goPIDs:           []int{1, 3, 5, 7},
		want:             []P{{PID: 1}, {PID: 3}, {PID: 5}, {PID: 7}},
		mock:             true,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mock {
				if runtime.GOOS != "linux" {
					t.Skip()
				}
				tempDir, err := os.MkdirTemp("", "")
				if err != nil {
					t.Errorf("failed to create temp dir: %v", err)
				}
				defer os.RemoveAll(tempDir)
				for _, p := range tc.input {
					os.Mkdir(filepath.Join(tempDir, strconv.Itoa(int(p.Pid))), 0o755)
					os.WriteFile(filepath.Join(tempDir, strconv.Itoa(int(p.Pid)), "stat"), []byte(
						`1440024 () R 0 1440024 0 34821 1440024 4194304 134 0 0 0 0 0 0 0 20 0 1 0 95120609 6746112 274 18446744073709551615 94467689938944 94467690036601 140724224197808 0 0 0 0 0 0 0 0 0 17 11 0 0 0 0 0 94467690068048 94467690071296 94467715629056 140724224199226 140724224199259 140724224199259 140724224204780 0`,
					), 0o644)
					os.WriteFile(filepath.Join(tempDir, strconv.Itoa(int(p.Pid)), "status"), []byte(
						`Name:
Umask:  0022
State:  R (running)
Tgid:   1440366
Ngid:   0
Pid:    1440366
PPid:   0
`,
					), 0o644)
				}
				os.Setenv("HOST_PROC", tempDir)
			}
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
	return func(pr *process.Process) (path, version string, agent, ok bool, err error) {
		for _, p := range goPIDs {
			if p == int(pr.Pid) {
				ok = true
				return
			}
		}
		return
	}
}

func fakeProcessesWithPIDs(pids ...int) []*process.Process {
	p := make([]*process.Process, 0, len(pids))
	for _, pid := range pids {
		p = append(p, &process.Process{Pid: int32(pid)})
	}
	return p
}

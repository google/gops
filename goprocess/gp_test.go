package goprocess

import (
	"os"
	"sync"
	"syscall"
	"testing"

	"github.com/keybase/go-ps"
)

func BenchmarkFindAll(b *testing.B) {
	for ii := 0; ii < b.N; ii++ {
		_ = FindAll()
	}
}

func TestEMFILE(t *testing.T) {
	pss, err := ps.Processes()
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(len(pss))

	for _, pr := range pss {
		pr := pr
		go func() {
			defer wg.Done()
			_, _, _, _, err := isGo(pr)
			if err != nil {
				if e, ok := err.(*os.PathError); ok && e.Err == syscall.EMFILE {
					t.Errorf("pid:%d got EMFILE error", pr.Pid())
				}
			}
		}()
	}

	wg.Wait()
}

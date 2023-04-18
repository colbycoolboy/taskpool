package pool

import (
	"errors"
	"time"
)

var (
	FullError  = errors.New("woker array full")
	EmptyError = errors.New("worker array empty")
)

type workArray struct {
	actives   []*poolWorker
	inactives []*poolWorker
}

func newWorkArray(size int) *workArray {
	return &workArray{
		// type length cap
		actives: make([]*poolWorker, 0, size),
	}
}

func (wa *workArray) len() int {
	return len(wa.actives)
}

func (wa *workArray) empty() bool {
	return wa.len() == 0
}

func (wa *workArray) putWorker(w *poolWorker) {
	wa.actives = append(wa.actives, w)
}

func (wa *workArray) getWorker() *poolWorker {
	end := wa.len()
	if end == 0 {
		return nil
	}
	w := wa.actives[end-1]
	// 这里不置为nil，那么会导致poolWorker一直被持有，无法回收
	wa.actives[end-1] = nil
	wa.actives = wa.actives[:end-1]
	return w
}

func (wa *workArray) getExpiredWorker(t time.Duration) []*poolWorker {
	l := wa.len()
	if l == 0 {
		return nil
	}
	activeTimeEnd := time.Now().Add(-1 * t)
	expiredIndex := wa.binarySearch(0, l-1, activeTimeEnd)
	if expiredIndex >= 0 {
		wa.inactives = append(wa.inactives, wa.actives[:expiredIndex+1]...)
		in := copy(wa.actives, wa.actives[expiredIndex+1:])
		for i := in; i < l; i++ {
			// 这里是需要置为nil,后续使用时发现为nil则直接当需要新建即可
			wa.actives[i] = nil
		}
		wa.actives = wa.actives[:in]
	}
	return wa.inactives
}

func (wa *workArray) binarySearch(l int, r int, activeTimeEnd time.Time) int {
	var mid int
	for l <= r {
		mid = l + ((r - l) >> 1)
		if activeTimeEnd.Before(wa.actives[mid].lastUseTime) {
			r = mid - 1
		} else {
			l = mid + 1
		}
	}
	return r
}

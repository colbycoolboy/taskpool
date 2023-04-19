package pool

import "time"

type poolWorker struct {
	pool        *Pool
	task        chan func()
	lastUseTime time.Time
}

func (w *poolWorker) execute() {
	w.pool.addRunning(1)
	go func() {
		defer func() {
			w.pool.addRunning(-1)
			w.pool.cache.Put(w)
			if p := recover(); p != nil {
				if h := w.pool.options.PanicHandler; h != nil {
					h(p)
				}
			}
			w.pool.cond.Signal()
		}()
		for f := range w.task {
			if f == nil {
				return
			}
			f()
			if cloudRecycle := w.pool.putWorker(w); !cloudRecycle {
				return
			}
		}
	}()

}

package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"sync/atomic"
)

var (
	OPEN  = 0
	CLOSE = 1
)

type Pool struct {
	running int32
	lock    sync.Locker
	workers *workArray
	state   int32
	cond    *sync.Cond
	waiting int32

	// 重启时可以使用，直接关闭无须操作
	stopHeartbeat context.CancelFunc

	options *Options
	cache   *sync.Pool
}

func BuildPool(options ...Option) (*Pool, error) {
	opts := loadOptions(options...)
	fmt.Println("pool size:", opts.Size)
	fmt.Println("wait size:", opts.MaxWaitTaskNum)
	fmt.Println("expire time:", opts.ExpireWorkerCleanInterval)

	if opts.Size <= 0 {
		return nil, fmt.Errorf("size less than 0")
	}

	pool := &Pool{
		lock:    &sync.Mutex{}, // 这里用cas最好
		options: opts,
	}

	pool.workers = newWorkArray(int(opts.Size))
	pool.cond = sync.NewCond(pool.lock)
	pool.cache.New = func() interface{} {
		return &poolWorker{
			pool: pool,
			task: make(chan func()),
		}
	}

	var ctx context.Context
	ctx, pool.stopHeartbeat = context.WithCancel(context.Background())
	if pool.options.ExpireWorkerCleanInterval != 0 {
		go pool.delExpiredWorker(ctx)
	}
	return pool, nil
}

func (p *Pool) Submit(task func()) error {
	if p.IsClose() {
		return fmt.Errorf("pool is close")
	}
	var w *poolWorker
	if w = p.getWorker(); w == nil {
		return fmt.Errorf("pool full")
	}
	w.task <- task
	return nil
}

func (p *Pool) Exit() {
	if !atomic.CompareAndSwapInt32(&p.state, int32(OPEN), int32(CLOSE)) {
		return
	}
retry:
	// 抢到锁的话，表示waiting的数量准确
	p.lock.Lock()
	if p.Waiting() > 0 {
		goto retry
	}
	p.workers.clean()
	p.lock.Unlock()
}

func (p *Pool) IsClose() bool {
	return atomic.LoadInt32(&p.state) == int32(CLOSE)
}

func (p *Pool) Cap() int {
	return int(atomic.LoadInt32(&p.options.Size))
}

func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

func (p *Pool) Waiting() int {
	return int(atomic.LoadInt32(&p.waiting))
}

func (p *Pool) delExpiredWorker(ctx context.Context) {
	hb := time.NewTimer(p.options.ExpireWorkerCleanInterval)

	defer func() {
		hb.Stop()
	}()

	for {
		select {
		case <-hb.C:
		case <-ctx.Done():
			return
		}
		p.lock.Lock()
		expairedWorkers := p.workers.getExpiredWorker(p.options.ExpireWorkerCleanInterval)
		p.lock.Unlock()

		for i := range expairedWorkers {
			// 发送nil后，task内部的goroutine会return
			expairedWorkers[i].task <- nil
			expairedWorkers[i] = nil
		}
		// 有可能所有都过期了
		if p.Running() > 0 || p.Waiting() > 0 {
			p.cond.Broadcast()
		}
	}
}

func (p *Pool) putWorker(worker *poolWorker) bool {
	if c := p.Cap(); p.Running() > c {
		p.cond.Broadcast()
		return false
	}
	worker.lastUseTime = time.Now()
	p.lock.Lock()
	p.workers.putWorker(worker)
	// 这里只需要通知一个
	p.cond.Signal()
	p.lock.Unlock()
	return true
}

func (p *Pool) getWorker() (w *poolWorker) {
	newWorkerAndRun := func() {
		w = p.cache.Get().(*poolWorker)
		w.execute()
	}
	p.lock.Lock()
	// 有情况不能defer
	// defer p.lock.Unlock()
	w = p.workers.getWorker()
	if w != nil {
		p.lock.Unlock()
		return
	} else if c := p.Cap(); c > p.Running() {
		p.lock.Unlock()
		newWorkerAndRun()
	} else {
		// 这里需要无限循环
		for {
			// retry:
			if p.Waiting() >= int(p.options.MaxWaitTaskNum) {
				p.lock.Unlock()
				return
			}
			p.addWaiting(1)
			p.cond.Wait()
			p.addWaiting(-1)

			// 这里有可能是被清理协程唤醒的，所以要判断是否还有正在运行的worker
			// 如果没有，则直接新建返回
			var newWorkerNum int
			if newWorkerNum = p.Running(); newWorkerNum == 0 {
				p.lock.Unlock()
				newWorkerAndRun()
				return
			}
			if w = p.workers.getWorker(); w == nil {
				if newWorkerNum < p.Cap() {
					p.lock.Unlock()
					newWorkerAndRun()
					return
				} else {
					//
					continue
				}
				// goto retry
			} else {
				break
			}
		}

		p.lock.Unlock()
	}
	return
}

func (p *Pool) addRunning(t int) {
	atomic.AddInt32(&p.running, int32(t))
}

func (p *Pool) addWaiting(t int) {
	atomic.AddInt32(&p.waiting, int32(t))
}

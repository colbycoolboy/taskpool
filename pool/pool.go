package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"sync/atomic"
)

type Pool struct {
	running int32
	lock    sync.Locker
	workers *workArray
	state   int32
	cond    *sync.Cond
	waiting int32

	stopHeartbeat context.CancelFunc

	options *Options
}

func BuildPool(options ...Option) (*Pool, error) {
	opts := loadOptions(options...)
	fmt.Println("pool size:%d", opts.Size)
	fmt.Println("wait size:%d", opts.MaxWaitTaskNum)
	fmt.Println("expire time:%d", opts.ExpireWorkerCleanInterval)

	if opts.Size <= 0 {
		return nil, fmt.Errorf("size less than 0")
	}

	pool := &Pool{
		lock:    &sync.Mutex{}, // 这里用cas最好
		options: opts,
	}

	pool.workers = newWorkArray(int(opts.Size))
	pool.cond = sync.NewCond(pool.lock)

	var ctx context.Context
	ctx, pool.stopHeartbeat = context.WithCancel(context.Background())
	if pool.options.ExpireWorkerCleanInterval != 0 {
		go pool.delExpiredWorker(ctx)
	}
	return pool, nil
}

func (p *Pool) Submit(task func()) error {
	var w *poolWorker
	if w = p.getWorker(); w == nil {
		return fmt.Errorf("pool full")
	}
	w.task <- task
	return nil
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
		w = &poolWorker{
			pool: p,
			task: make(chan func(), 0),
		}
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

/*
var (
	foodOk   = false
	foodName = ""
	rwMutex  = &sync.RWMutex{}
	cond     = sync.NewCond(rwMutex.RLocker())
)

func makeFood() {
	fmt.Print("start make food")
	time.Sleep(3 * time.Second)
	foodOk = true
	foodName = "a"
	fmt.Print("food ok")
	cond.Broadcast()
}

func waitToEat() {
	cond.L.Lock()
	defer cond.L.Unlock()
	for !foodOk {
		cond.Wait()
	}
	fmt.Printf("eat food :%s", foodName)
}

func main() {
	fmt.Println("vim-go")
	for i := 0; i <= 3; i++ {
		go waitToEat()
	}
	go makeFood()
	time.Sleep(10 * time.Second)
}
*/

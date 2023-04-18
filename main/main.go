package main

import (
	"fmt"
	"time"

	"taskpool/pool"
)

func main() {
	fmt.Println("vim-go")
	// test zero value
	// var c chan int
	// fmt.Printf("%v", c)
	taskPool, err := pool.BuildPool(pool.Size(5),
		pool.MaxWaitTaskNum(5), pool.ExpreWorkerCleanInterval(10*time.Second),
		pool.PanicHandler(func(i interface{}) {
			fmt.Printf("execute error:%v", i)
		}))

	if err != nil {
		fmt.Printf("init error:%v", err)
		panic(err)
	}
	fmt.Println("-----------------")
	for i := 0; i < 10; i++ {
		a := i
		fn := func() {
			time.Sleep(3 * time.Second)
			fmt.Printf("run task :%d\n", a)
		}
		go func() {
			if err := taskPool.Submit(fn); err != nil {
				fmt.Printf("add error %v\n", err)
			}
		}()
	}
	fmt.Println("add 10 task")
	time.Sleep(1 * time.Second)
	for i := 0; i < 5; i++ {
		a := i
		fn := func() {
			time.Sleep(3 * time.Second)
			fmt.Printf("run task :%d\n", a+10)
		}
		go func() {
			if err := taskPool.Submit(fn); err != nil {
				fmt.Printf("add error %v\n", err)
			}
		}()
	}
	time.Sleep(10 * time.Second)
}

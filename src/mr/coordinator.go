package mr

/**
Data race may occur when you execute `go run -race mrcoordinator.go pg-*.txt`. I think race will not happen in fact
*/

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)
import "net"
import "os"
import "net/rpc"
import "net/http"

type Coordinator struct {
	// map just for the random visit
	MapTaskMap        map[int]*MapTask
	ReduceTaskMap     map[int]*ReduceTask
	ActiveMapTasks    []*MapTask
	ActiveReduceTasks []*ReduceTask
	ReduceN           int
	mutex             sync.Mutex
}

type MapTask struct {
	TaskId   int
	FileName string
	Status   int32 //0:NOTRUN 1:RUNNING 2:FINISHED
}

type ReduceTask struct {
	TaskId   int
	FileName []string
	Status   int32
}

type TaskStatus int32

const (
	NOTRUN   TaskStatus = 0
	RUNNNING TaskStatus = 1
	FINISHED TaskStatus = 2
)

// HandleWorker will return the task for worker. it will start a crash check thread for every task
func (c *Coordinator) HandleWorker(args *CoordinatorArgs, reply *CoordinatorReply) error {
	c.mutex.Lock()
	mapTask := getMapTask(c.ActiveMapTasks)
	if mapTask != nil {
		mapTask.Status = 1

		reply.TaskId = mapTask.TaskId
		reply.FileName = []string{mapTask.FileName}
		reply.TaskType = "MAP"
		reply.ReduceN = c.ReduceN

		go func(mapTask *MapTask) {
			time.Sleep(10 * time.Second)
			atomic.CompareAndSwapInt32(&mapTask.Status, 1, 0)
		}(mapTask)
	} else if getReduceTask(c.ActiveMapTasks, c.ActiveReduceTasks) != nil {
		reduceTask := getReduceTask(c.ActiveMapTasks, c.ActiveReduceTasks)
		reduceTask.Status = 1

		reply.TaskId = reduceTask.TaskId
		reply.FileName = reduceTask.FileName
		reply.TaskType = "REDUCE"
		reply.ReduceN = c.ReduceN

		go func(reduceTask *ReduceTask) {
			time.Sleep(10 * time.Second)
			atomic.CompareAndSwapInt32(&reduceTask.Status, 1, 0)
		}(reduceTask)
	}
	c.mutex.Unlock()
	return nil
}

// Will return nil when task is running (may fail then)
func getMapTask(ActiveMapTasks []*MapTask) *MapTask {
	for _, value := range ActiveMapTasks {
		if value.Status == 0 {
			return value
		}
	}
	return nil
}

// return the NOTRUN reduce when all the map task finish
func getReduceTask(ActiveMapTasks []*MapTask, ActiveReduceTasks []*ReduceTask) *ReduceTask {
	mapFinished := true
	for _, value := range ActiveMapTasks {
		if value.Status != 2 {
			mapFinished = false
			break
		}
	}
	if mapFinished {
		for _, value := range ActiveReduceTasks {
			if value.Status == 0 {
				return value
			}
		}
	}
	return nil
}

// HandleFinish will set status to FINISHED with CAS
func (c *Coordinator) HandleFinish(args *CoordinatorFinishArgs, reply *CoordinatorFinishReply) error {
	if args.TaskType == "MAP" {
		mapTask := c.MapTaskMap[args.Id]
		atomic.CompareAndSwapInt32(&mapTask.Status, 1, 2)
	} else if args.TaskType == "REDUCE" {
		reduceTask := c.ReduceTaskMap[args.Id]
		atomic.CompareAndSwapInt32(&reduceTask.Status, 1, 2)
	}
	return nil
}

func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {

	for _, task := range c.ActiveReduceTasks {
		if task.Status != 2 {
			return false
		}
	}

	return true
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {

	MapTaskMap := make(map[int]*MapTask)
	ReduceTaskMap := make(map[int]*ReduceTask)
	var ActiveMapTasks []*MapTask
	var ActiveReduceTasks []*ReduceTask

	for index, f := range files {
		mapTask := &MapTask{
			TaskId:   index + 1,
			FileName: f,
			Status:   0,
		}
		ActiveMapTasks = append(ActiveMapTasks, mapTask)
		MapTaskMap[mapTask.TaskId] = mapTask
	}

	for i := 0; i < nReduce; i++ {
		var fileName []string
		for j := 0; j < len(files); j++ {
			fileName = append(fileName, fmt.Sprintf("mr-%d-%d", j+1, i+1))
		}
		reduceTask := &ReduceTask{
			TaskId:   i + 1,
			FileName: fileName,
			Status:   0,
		}
		ActiveReduceTasks = append(ActiveReduceTasks, reduceTask)
		ReduceTaskMap[reduceTask.TaskId] = reduceTask
	}

	c := Coordinator{
		MapTaskMap:        MapTaskMap,
		ReduceTaskMap:     ReduceTaskMap,
		ActiveMapTasks:    ActiveMapTasks,
		ActiveReduceTasks: ActiveReduceTasks,
		ReduceN:           nReduce,
	}

	c.server()
	return &c
}
